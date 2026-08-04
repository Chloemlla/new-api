package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/response_cache_setting"

	"github.com/gin-gonic/gin"
)

const (
	// responseCacheKeyPrefix 缓存键前缀，用于 Redis 扫描清理
	responseCacheKeyPrefix = "response_cache:"
	// responseCacheMemoryLimit 内存模式下的最大缓存条目数
	responseCacheMemoryLimit = 10000
	// responseCacheDefaultMaxBytes 未配置时的默认最大响应字节数（1MB）
	responseCacheDefaultMaxBytes = 1 << 20
	// responseCacheDefaultTTL 未配置时的默认缓存有效期
	responseCacheDefaultTTL = 60 * time.Second
)

var (
	responseCacheHits   atomic.Int64
	responseCacheMisses atomic.Int64
)

// responseCacheEntry 缓存响应条目
type responseCacheEntry struct {
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

// memoryCacheItem 内存缓存条目
type memoryCacheItem struct {
	entry     responseCacheEntry
	expiresAt time.Time
}

// memoryResponseCache 内存缓存存储，仅在 Redis 未启用时使用
type memoryResponseCache struct {
	mu      sync.Mutex
	entries map[string]memoryCacheItem
}

var responseCacheMemory = &memoryResponseCache{
	entries: make(map[string]memoryCacheItem),
}

func (m *memoryResponseCache) get(key string) (responseCacheEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.entries[key]
	if !ok {
		return responseCacheEntry{}, false
	}
	if time.Now().After(item.expiresAt) {
		delete(m.entries, key)
		return responseCacheEntry{}, false
	}
	return item.entry, true
}

func (m *memoryResponseCache) set(key string, entry responseCacheEntry, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 缓存满时先清理过期条目
	if len(m.entries) >= responseCacheMemoryLimit {
		now := time.Now()
		for k, v := range m.entries {
			if now.After(v.expiresAt) {
				delete(m.entries, k)
			}
		}
	}
	// 仍然满则淘汰最早过期的条目
	if len(m.entries) >= responseCacheMemoryLimit {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range m.entries {
			if oldestKey == "" || v.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiresAt
			}
		}
		if oldestKey != "" {
			delete(m.entries, oldestKey)
		}
	}
	m.entries[key] = memoryCacheItem{
		entry:     entry,
		expiresAt: time.Now().Add(ttl),
	}
}

func (m *memoryResponseCache) size() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func (m *memoryResponseCache) purge() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := len(m.entries)
	m.entries = make(map[string]memoryCacheItem)
	return count
}

func responseCacheGet(key string) (responseCacheEntry, bool) {
	var entry responseCacheEntry
	if common.RedisEnabled && common.RDB != nil {
		val, err := common.RedisGet(key)
		if err != nil {
			return entry, false
		}
		if err := common.UnmarshalJsonStr(val, &entry); err != nil {
			return entry, false
		}
		return entry, true
	}
	return responseCacheMemory.get(key)
}

func responseCacheSet(key string, entry responseCacheEntry, ttl time.Duration) {
	if common.RedisEnabled && common.RDB != nil {
		data, err := common.Marshal(entry)
		if err != nil {
			return
		}
		_ = common.RedisSet(key, string(data), ttl)
		return
	}
	responseCacheMemory.set(key, entry, ttl)
}

// responseCacheWriter 包装 gin.ResponseWriter，捕获响应内容用于缓存。
// 数据同时转发给真正的 writer，保证客户端实时收到响应。
// 注意：不能用 Flush() 判断是否流式——relay 的非流式响应也会调用 Flush()
// （见 service.IOCopyBytesGracefully），流式判定改为依赖 text/event-stream 内容类型。
type responseCacheWriter struct {
	gin.ResponseWriter
	buf       bytes.Buffer
	maxBytes  int
	oversized bool
}

func (w *responseCacheWriter) Write(data []byte) (int, error) {
	if !w.oversized {
		if w.maxBytes > 0 && w.buf.Len()+len(data) > w.maxBytes {
			// 超过上限后停止缓冲，避免为一条无法缓存的大响应分配过多内存
			w.oversized = true
			w.buf.Reset()
		} else {
			w.buf.Write(data)
		}
	}
	return w.ResponseWriter.Write(data)
}

func (w *responseCacheWriter) WriteString(s string) (int, error) {
	if !w.oversized {
		if w.maxBytes > 0 && w.buf.Len()+len(s) > w.maxBytes {
			w.oversized = true
			w.buf.Reset()
		} else {
			w.buf.WriteString(s)
		}
	}
	// gin 的 WriteString 不会主动写出响应头，这里先刷新
	w.ResponseWriter.WriteHeaderNow()
	return w.ResponseWriter.WriteString(s)
}

// ResponseCache 请求/响应缓存中间件。
//
// 对完全相同的 API 请求（方法、路径、查询参数、请求体、调用身份一致）返回缓存的响应，
// 命中时不会经过上游渠道、不产生调用扣费。Redis 可用时使用 Redis 存储，否则退化为内存存储。
// 缓存命中/未命中通过响应头 X-Cache 标识（HIT/MISS）。
func ResponseCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := response_cache_setting.GetResponseCacheSetting()
		if cfg == nil || !cfg.Enabled {
			c.Next()
			return
		}

		method := c.Request.Method
		if method != http.MethodGet && method != http.MethodPost {
			c.Next()
			return
		}

		if requestBypassesCache(c) {
			c.Next()
			return
		}

		// 仅缓存 JSON 请求体；文件上传（multipart）等不缓存
		if method == http.MethodPost && !isJSONRequest(c) {
			c.Next()
			return
		}

		body, ok := readRequestBodyForCache(c)
		if !ok {
			c.Next()
			return
		}

		if method == http.MethodPost && cfg.OnlyDeterministic && !isDeterministicRequest(body) {
			c.Next()
			return
		}

		ttl := time.Duration(cfg.TTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = responseCacheDefaultTTL
		}

		key := buildResponseCacheKey(c, body)

		if entry, found := responseCacheGet(key); found {
			responseCacheHits.Add(1)
			writeCachedResponse(c, entry)
			return
		}
		responseCacheMisses.Add(1)

		maxBytes := cfg.MaxResponseSizeKB * 1024
		if maxBytes <= 0 {
			maxBytes = responseCacheDefaultMaxBytes
		}
		writer := &responseCacheWriter{
			ResponseWriter: c.Writer,
			maxBytes:       maxBytes,
		}
		writer.Header().Set("X-Cache", "MISS")
		c.Writer = writer

		c.Next()

		if shouldCacheResponse(writer) {
			responseCacheSet(key, responseCacheEntry{
				StatusCode:  writer.Status(),
				ContentType: writer.Header().Get("Content-Type"),
				Body:        writer.buf.Bytes(),
			}, ttl)
		}
	}
}

// requestBypassesCache 客户端通过 Cache-Control/Pragma 显式要求绕过缓存
func requestBypassesCache(c *gin.Context) bool {
	cacheControl := c.GetHeader("Cache-Control")
	for _, part := range strings.Split(cacheControl, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "no-store", "no-cache":
			return true
		}
	}
	return strings.EqualFold(c.GetHeader("Pragma"), "no-cache")
}

func isJSONRequest(c *gin.Context) bool {
	return strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json")
}

// readRequestBodyForCache 读取请求体用于生成缓存键。
// GET 请求没有请求体，直接返回成功。
func readRequestBodyForCache(c *gin.Context) ([]byte, bool) {
	if c.Request.Method == http.MethodGet {
		return nil, true
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, false
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, false
	}
	return body, true
}

// buildResponseCacheKey 基于方法、路径、查询参数、请求体和调用身份生成缓存键。
// 身份（用户/令牌）参与哈希，避免不同调用方之间共享缓存导致数据隔离问题。
func buildResponseCacheKey(c *gin.Context, body []byte) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00%s\x00%s\x00", c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery)
	fmt.Fprintf(hash, "user:%d token:%d\x00", c.GetInt("id"), c.GetInt("token_id"))
	hash.Write(body)
	return responseCacheKeyPrefix + hex.EncodeToString(hash.Sum(nil))
}

// cachePolicyRequest 用于判定请求是否确定性（只解析需要的字段，避免解析整个请求体）
type cachePolicyRequest struct {
	Stream           bool                  `json:"stream"`
	Temperature      *float64              `json:"temperature"`
	TopP             *float64              `json:"top_p"`
	N                *int                  `json:"n"`
	GenerationConfig *cachePolicyGenConfig `json:"generationConfig"`
}

// cachePolicyGenConfig Gemini 请求的生成配置
type cachePolicyGenConfig struct {
	Temperature    *float64 `json:"temperature"`
	TopP           *float64 `json:"topP"`
	CandidateCount *int     `json:"candidateCount"`
}

// isDeterministicRequest 判断请求是否为确定性请求，只有确定性请求才允许缓存。
// 解析失败时保守地返回 false（不缓存）。
func isDeterministicRequest(body []byte) bool {
	if len(body) == 0 {
		return true
	}
	var req cachePolicyRequest
	if err := common.Unmarshal(body, &req); err != nil {
		return false
	}
	if req.Stream {
		return false
	}

	temperature := req.Temperature
	topP := req.TopP
	n := req.N
	if req.GenerationConfig != nil {
		if req.GenerationConfig.Temperature != nil {
			temperature = req.GenerationConfig.Temperature
		}
		if req.GenerationConfig.TopP != nil {
			topP = req.GenerationConfig.TopP
		}
		if req.GenerationConfig.CandidateCount != nil {
			n = req.GenerationConfig.CandidateCount
		}
	}
	if temperature != nil && *temperature > 0 {
		return false
	}
	if topP != nil && *topP < 1.0 {
		return false
	}
	if n != nil && *n != 1 {
		return false
	}
	return true
}

// shouldCacheResponse 判断响应是否可缓存：非流式、2xx、非空且未超过大小上限
func shouldCacheResponse(w *responseCacheWriter) bool {
	if w.oversized || w.buf.Len() == 0 {
		return false
	}
	status := w.Status()
	if status < 200 || status > 299 {
		return false
	}
	if strings.HasPrefix(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		return false
	}
	return true
}

// writeCachedResponse 直接写出缓存的响应并终止请求链
func writeCachedResponse(c *gin.Context, entry responseCacheEntry) {
	c.Header("X-Cache", "HIT")
	c.Data(entry.StatusCode, entry.ContentType, entry.Body)
	c.Abort()
}

// ResponseCacheStats 请求/响应缓存统计信息
type ResponseCacheStats struct {
	Enabled     bool  `json:"enabled"`
	Hits        int64 `json:"hits"`
	Misses      int64 `json:"misses"`
	MemoryItems int   `json:"memory_items"`
}

// GetResponseCacheStats 获取缓存统计信息
func GetResponseCacheStats() ResponseCacheStats {
	cfg := response_cache_setting.GetResponseCacheSetting()
	return ResponseCacheStats{
		Enabled:     cfg != nil && cfg.Enabled,
		Hits:        responseCacheHits.Load(),
		Misses:      responseCacheMisses.Load(),
		MemoryItems: responseCacheMemory.size(),
	}
}

// ClearResponseCache 清空所有缓存条目，返回删除数量。
// Redis 模式下仅删除本中间件使用的前缀键。
func ClearResponseCache() (int, error) {
	memoryDeleted := responseCacheMemory.purge()
	if !common.RedisEnabled || common.RDB == nil {
		return memoryDeleted, nil
	}

	ctx := context.Background()
	var keys []string
	iter := common.RDB.Scan(ctx, 0, responseCacheKeyPrefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return memoryDeleted, err
	}
	if len(keys) == 0 {
		return memoryDeleted, nil
	}
	if err := common.RDB.Del(ctx, keys...).Err(); err != nil {
		return memoryDeleted, err
	}
	return memoryDeleted + len(keys), nil
}
