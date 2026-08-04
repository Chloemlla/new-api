package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/response_cache_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enableResponseCacheMemory 启用缓存并强制使用内存存储，测试结束后还原
func enableResponseCacheMemory(t *testing.T, overrides map[string]any) {
	t.Helper()
	cfg := response_cache_setting.GetResponseCacheSetting()
	previous := *cfg
	*cfg = response_cache_setting.ResponseCacheSetting{
		Enabled:           true,
		TTLSeconds:        60,
		MaxResponseSizeKB: 1024,
		OnlyDeterministic: true,
	}
	applyResponseCacheOverrides(cfg, overrides)

	previousEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = false
	common.RDB = nil

	t.Cleanup(func() {
		*cfg = previous
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
		_, _ = ClearResponseCache()
		responseCacheHits.Store(0)
		responseCacheMisses.Store(0)
	})
}

// useResponseCacheMiniRedis 启用缓存并切换到 Redis 存储
func useResponseCacheMiniRedis(t *testing.T, overrides map[string]any) *miniredis.Miniredis {
	t.Helper()
	cfg := response_cache_setting.GetResponseCacheSetting()
	previous := *cfg
	*cfg = response_cache_setting.ResponseCacheSetting{
		Enabled:           true,
		TTLSeconds:        60,
		MaxResponseSizeKB: 1024,
		OnlyDeterministic: true,
	}
	applyResponseCacheOverrides(cfg, overrides)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())

	previousEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = redisClient

	t.Cleanup(func() {
		_ = redisClient.Close()
		*cfg = previous
		common.RedisEnabled = previousEnabled
		common.RDB = previousRDB
		responseCacheHits.Store(0)
		responseCacheMisses.Store(0)
	})
	return redisServer
}

func applyResponseCacheOverrides(cfg *response_cache_setting.ResponseCacheSetting, overrides map[string]any) {
	if overrides == nil {
		return
	}
	if v, ok := overrides["ttl_seconds"]; ok {
		cfg.TTLSeconds = v.(int)
	}
	if v, ok := overrides["max_response_size_kb"]; ok {
		cfg.MaxResponseSizeKB = v.(int)
	}
	if v, ok := overrides["only_deterministic"]; ok {
		cfg.OnlyDeterministic = v.(bool)
	}
}

// newResponseCacheRouter 构造带身份注入的测试路由
// identity 返回在缓存中间件之前写入 ctx 的身份
func newResponseCacheRouter(identity func(c *gin.Context), handler gin.HandlerFunc) (*gin.Engine, *atomic.Int32) {
	gin.SetMode(gin.TestMode)
	var calls atomic.Int32
	router := gin.New()
	router.POST("/v1/chat/completions",
		func(c *gin.Context) {
			if identity != nil {
				identity(c)
			}
			c.Next()
		},
		ResponseCache(),
		func(c *gin.Context) {
			calls.Add(1)
			handler(c)
		},
	)
	return router, &calls
}

func performResponseCachePost(router http.Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestResponseCacheMemoryCacheHitSkipsHandler(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	first := performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, nil)
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, "MISS", first.Header().Get("X-Cache"))
	assert.JSONEq(t, `{"result":"hello"}`, first.Body.String())

	second := performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, nil)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, "HIT", second.Header().Get("X-Cache"))
	assert.JSONEq(t, `{"result":"hello"}`, second.Body.String())
	assert.Equal(t, int32(1), calls.Load(), "cache hit must not invoke the handler again")
}

func TestResponseCacheMemoryDifferentBodiesProduceDifferentEntries(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, nil)
	performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"bye"}]}`, nil)
	assert.Equal(t, int32(2), calls.Load(), "different request bodies must not share a cache entry")
}

func TestResponseCacheMemoryDifferentIdentitiesAreIsolated(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	var currentUser int
	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", currentUser)
			c.Set("token_id", 100+currentUser)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	currentUser = 1
	performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, nil)
	currentUser = 2
	performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, nil)
	assert.Equal(t, int32(2), calls.Load(), "different callers must not share cached responses")
}

func TestResponseCacheMemoryNonDeterministicRequestNotCached(t *testing.T) {
	enableResponseCacheMemory(t, map[string]any{"only_deterministic": true})

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"model":"gpt-4o","temperature":0.7,"messages":[{"role":"user","content":"hi"}]}`
	performResponseCachePost(router, body, nil)
	performResponseCachePost(router, body, nil)
	assert.Equal(t, int32(2), calls.Load(), "sampling requests must not be cached when only_deterministic is enabled")
}

func TestResponseCacheMemoryGeminiGenerationConfigRespected(t *testing.T) {
	enableResponseCacheMemory(t, map[string]any{"only_deterministic": true})

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"temperature":0.9,"candidateCount":1}}`
	performResponseCachePost(router, body, nil)
	performResponseCachePost(router, body, nil)
	assert.Equal(t, int32(2), calls.Load(), "gemini requests with sampling must not be cached")
}

func TestResponseCacheStreamResponseNotCached(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			// 上游返回 SSE（即使请求未显式 stream），内容类型判定为流式则不缓存
			c.Header("Content-Type", "text/event-stream")
			c.Writer.WriteHeaderNow()
			_, _ = c.Writer.WriteString("data: [DONE]\n\n")
			c.Writer.Flush()
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	performResponseCachePost(router, body, nil)
	performResponseCachePost(router, body, nil)
	assert.Equal(t, int32(2), calls.Load(), "text/event-stream responses must not be cached")
}

func TestResponseCacheStreamRequestNotCached(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	performResponseCachePost(router, body, nil)
	performResponseCachePost(router, body, nil)
	assert.Equal(t, int32(2), calls.Load(), "streaming requests must not be cached")
}

// TestResponseCacheNonStreamingFlushIsCached 回归测试：
// relay 的非流式响应在写完后会调用 Flush()（service.IOCopyBytesGracefully），
// 这类响应必须仍然可以被缓存，不能因为出现过 Flush 而被误判为流式。
func TestResponseCacheNonStreamingFlushIsCached(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			// 模拟 IOCopyBytesGracefully：复制上游响应头、写状态码、写 body、最后 Flush
			c.Header("Content-Type", "application/json")
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.WriteString(`{"result":"hello"}`)
			c.Writer.Flush()
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	assert.Equal(t, "MISS", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))
	second := performResponseCachePost(router, body, nil)
	assert.Equal(t, "HIT", second.Header().Get("X-Cache"))
	assert.JSONEq(t, `{"result":"hello"}`, second.Body.String())
	assert.Equal(t, int32(1), calls.Load(), "non-streaming flushed responses must be cached")
}

func TestResponseCacheMemoryNoStoreDirectiveBypassesCache(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	headers := map[string]string{"Cache-Control": "no-store"}
	performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, headers)
	performResponseCachePost(router, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`, headers)
	assert.Equal(t, int32(2), calls.Load(), "Cache-Control: no-store must bypass the cache")
}

func TestResponseCacheMemoryResponseSizeLimit(t *testing.T) {
	enableResponseCacheMemory(t, map[string]any{"max_response_size_kb": 1})

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			// 约 2KB 的响应，超过 1KB 上限
			c.String(http.StatusOK, strings.Repeat("x", 2048))
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	performResponseCachePost(router, body, nil)
	performResponseCachePost(router, body, nil)
	assert.Equal(t, int32(2), calls.Load(), "responses over the size limit must not be cached")
}

func TestResponseCacheMemoryErrorResponseNotCached(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	performResponseCachePost(router, body, nil)
	performResponseCachePost(router, body, nil)
	assert.Equal(t, int32(2), calls.Load(), "error responses must not be cached")
}

func TestResponseCacheMemoryPurgeClearsEntries(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	router, _ := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	assert.Equal(t, "MISS", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))
	assert.Equal(t, "HIT", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))

	deleted, err := ClearResponseCache()
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	assert.Equal(t, "MISS", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))
}

func TestResponseCacheMemoryTTLExpiry(t *testing.T) {
	enableResponseCacheMemory(t, map[string]any{"ttl_seconds": 3600})

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	performResponseCachePost(router, body, nil)
	assert.Equal(t, "HIT", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))

	// 手动把缓存条目改成已过期，模拟 TTL 到期
	responseCacheMemory.mu.Lock()
	for k, v := range responseCacheMemory.entries {
		v.expiresAt = time.Now().Add(-time.Second)
		responseCacheMemory.entries[k] = v
	}
	responseCacheMemory.mu.Unlock()

	assert.Equal(t, "MISS", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))
	assert.Equal(t, int32(2), calls.Load())
}

func TestResponseCacheRedisBackend(t *testing.T) {
	redisServer := useResponseCacheMiniRedis(t, nil)

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	first := performResponseCachePost(router, body, nil)
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, "MISS", first.Header().Get("X-Cache"))

	second := performResponseCachePost(router, body, nil)
	assert.Equal(t, "HIT", second.Header().Get("X-Cache"))
	assert.JSONEq(t, `{"result":"hello"}`, second.Body.String())
	assert.Equal(t, int32(1), calls.Load())

	keys := redisServer.Keys()
	require.Len(t, keys, 1)
	assert.True(t, strings.HasPrefix(keys[0], responseCacheKeyPrefix))
	assert.Equal(t, time.Duration(60)*time.Second, redisServer.TTL(keys[0]))
}

func TestResponseCacheRedisPurgeDeletesKeys(t *testing.T) {
	redisServer := useResponseCacheMiniRedis(t, nil)

	router, _ := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	performResponseCachePost(router, body, nil)
	assert.Equal(t, "HIT", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))

	deleted, err := ClearResponseCache()
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)
	assert.Empty(t, redisServer.Keys())

	assert.Equal(t, "MISS", performResponseCachePost(router, body, nil).Header().Get("X-Cache"))
}

func TestResponseCacheGETRequestsCached(t *testing.T) {
	enableResponseCacheMemory(t, nil)

	gin.SetMode(gin.TestMode)
	var calls atomic.Int32
	router := gin.New()
	router.GET("/v1/models",
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
			c.Next()
		},
		ResponseCache(),
		func(c *gin.Context) {
			calls.Add(1)
			c.JSON(http.StatusOK, gin.H{"data": []string{"gpt-4o"}})
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	router.ServeHTTP(recorder, request)
	assert.Equal(t, "MISS", recorder.Header().Get("X-Cache"))

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	router.ServeHTTP(recorder, request)
	assert.Equal(t, "HIT", recorder.Header().Get("X-Cache"))
	assert.JSONEq(t, `{"data":["gpt-4o"]}`, recorder.Body.String())
	assert.Equal(t, int32(1), calls.Load())
}

func TestResponseCacheDisabledPassesThrough(t *testing.T) {
	cfg := response_cache_setting.GetResponseCacheSetting()
	previous := *cfg
	*cfg = response_cache_setting.ResponseCacheSetting{Enabled: false}
	t.Cleanup(func() { *cfg = previous })

	router, calls := newResponseCacheRouter(
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set("token_id", 1)
		},
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"result": "hello"})
		},
	)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	first := performResponseCachePost(router, body, nil)
	assert.Empty(t, first.Header().Get("X-Cache"), "disabled middleware must not add X-Cache header")
	performResponseCachePost(router, body, nil)
	assert.Equal(t, int32(2), calls.Load())
}
