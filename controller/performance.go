package controller

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// PerformanceStats 性能统计信息
type PerformanceStats struct {
	// 缓存统计
	CacheStats common.DiskCacheStats `json:"cache_stats"`
	// 系统内存统计
	MemoryStats MemoryStats `json:"memory_stats"`
	// 磁盘缓存目录信息
	DiskCacheInfo DiskCacheInfo `json:"disk_cache_info"`
	// 磁盘空间信息
	DiskSpaceInfo common.DiskSpaceInfo `json:"disk_space_info"`
	// 配置信息
	Config PerformanceConfig `json:"config"`
}

// MemoryStats 内存统计
type MemoryStats struct {
	// 已分配内存（字节）
	Alloc uint64 `json:"alloc"`
	// 总分配内存（字节）
	TotalAlloc uint64 `json:"total_alloc"`
	// 系统内存（字节）
	Sys uint64 `json:"sys"`
	// GC 次数
	NumGC uint32 `json:"num_gc"`
	// Goroutine 数量
	NumGoroutine int `json:"num_goroutine"`
}

// DiskCacheInfo 磁盘缓存目录信息
type DiskCacheInfo struct {
	// 缓存目录路径
	Path string `json:"path"`
	// 目录是否存在
	Exists bool `json:"exists"`
	// 文件数量
	FileCount int `json:"file_count"`
	// 总大小（字节）
	TotalSize int64 `json:"total_size"`
}

// PerformanceConfig 性能配置
type PerformanceConfig struct {
	// 是否启用磁盘缓存
	DiskCacheEnabled bool `json:"disk_cache_enabled"`
	// 磁盘缓存阈值（MB）
	DiskCacheThresholdMB int `json:"disk_cache_threshold_mb"`
	// 磁盘缓存最大大小（MB）
	DiskCacheMaxSizeMB int `json:"disk_cache_max_size_mb"`
	// 磁盘缓存路径
	DiskCachePath string `json:"disk_cache_path"`
	// 是否在容器中运行
	IsRunningInContainer bool `json:"is_running_in_container"`

	// MonitorEnabled 是否启用性能监控
	MonitorEnabled bool `json:"monitor_enabled"`
	// MonitorCPUThreshold CPU 使用率阈值（%）
	MonitorCPUThreshold int `json:"monitor_cpu_threshold"`
	// MonitorMemoryThreshold 内存使用率阈值（%）
	MonitorMemoryThreshold int `json:"monitor_memory_threshold"`
	// MonitorDiskThreshold 磁盘使用率阈值（%）
	MonitorDiskThreshold int `json:"monitor_disk_threshold"`
}

// GetPerformanceStats 获取性能统计信息
// GetPerformanceStats 获取性能统计信息
func GetPerformanceStats(c *gin.Context) {
	// 不再每次获取统计都全量扫描磁盘，依赖原子计数器保证性能
	// 仅在系统启动或显式清理时同步
	cacheStats := common.GetDiskCacheStats()

	// 获取内存统计
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 获取磁盘缓存目录信息
	diskCacheInfo := getDiskCacheInfo()

	// 获取配置信息
	diskConfig := common.GetDiskCacheConfig()
	monitorConfig := common.GetPerformanceMonitorConfig()
	config := PerformanceConfig{
		DiskCacheEnabled:       diskConfig.Enabled,
		DiskCacheThresholdMB:   diskConfig.ThresholdMB,
		DiskCacheMaxSizeMB:     diskConfig.MaxSizeMB,
		DiskCachePath:          diskConfig.Path,
		IsRunningInContainer:   common.IsRunningInContainer(),
		MonitorEnabled:         monitorConfig.Enabled,
		MonitorCPUThreshold:    monitorConfig.CPUThreshold,
		MonitorMemoryThreshold: monitorConfig.MemoryThreshold,
		MonitorDiskThreshold:   monitorConfig.DiskThreshold,
	}

	// 获取磁盘空间信息
	// 使用缓存的系统状态，避免频繁调用系统 API
	systemStatus := common.GetSystemStatus()
	diskSpaceInfo := common.DiskSpaceInfo{
		UsedPercent: systemStatus.DiskUsage,
	}
	// 如果需要详细信息，可以按需获取，或者扩展 SystemStatus
	// 这里为了保持接口兼容性，我们仍然调用 GetDiskSpaceInfo，但注意这可能会有性能开销
	// 考虑到 GetPerformanceStats 是管理接口，频率较低，直接调用是可以接受的
	// 但为了一致性，我们也可以考虑从 SystemStatus 中获取部分信息
	diskSpaceInfo = common.GetDiskSpaceInfo()

	stats := PerformanceStats{
		CacheStats: cacheStats,
		MemoryStats: MemoryStats{
			Alloc:        memStats.Alloc,
			TotalAlloc:   memStats.TotalAlloc,
			Sys:          memStats.Sys,
			NumGC:        memStats.NumGC,
			NumGoroutine: runtime.NumGoroutine(),
		},
		DiskCacheInfo: diskCacheInfo,
		DiskSpaceInfo: diskSpaceInfo,
		Config:        config,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// ClearDiskCache 清理不活跃的磁盘缓存
func ClearDiskCache(c *gin.Context) {
	// 清理超过 10 分钟未使用的缓存文件
	// 10 分钟是一个安全的阈值，确保正在进行的请求不会被误删
	err := common.CleanupOldDiskCacheFiles(10 * time.Minute)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "不活跃的磁盘缓存已清理",
	})
}

// ResetPerformanceStats 重置性能统计
func ResetPerformanceStats(c *gin.Context) {
	common.ResetDiskCacheStats()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "统计信息已重置",
	})
}

// ForceGC 强制执行 GC
func ForceGC(c *gin.Context) {
	runtime.GC()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "GC 已执行",
	})
}

// LogFileInfo 日志文件信息
type LogFileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// LogFilesResponse 日志文件列表响应
type LogFilesResponse struct {
	LogDir     string        `json:"log_dir"`
	Enabled    bool          `json:"enabled"`
	FileCount  int           `json:"file_count"`
	TotalSize  int64         `json:"total_size"`
	OldestTime *time.Time    `json:"oldest_time,omitempty"`
	NewestTime *time.Time    `json:"newest_time,omitempty"`
	Files      []LogFileInfo `json:"files"`
}

// getLogFiles 读取日志目录中的日志文件列表
func getLogFiles() ([]LogFileInfo, error) {
	if *common.LogDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(*common.LogDir)
	if err != nil {
		return nil, err
	}
	var files []LogFileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "oneapi-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, LogFileInfo{
			Name:    name,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	// 按文件名降序排列（最新在前）
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name
	})
	return files, nil
}

// GetLogFiles 获取日志文件列表
func GetLogFiles(c *gin.Context) {
	if *common.LogDir == "" {
		common.ApiSuccess(c, LogFilesResponse{Enabled: false})
		return
	}
	files, err := getLogFiles()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var totalSize int64
	var oldest, newest time.Time
	for i, f := range files {
		totalSize += f.Size
		if i == 0 || f.ModTime.Before(oldest) {
			oldest = f.ModTime
		}
		if i == 0 || f.ModTime.After(newest) {
			newest = f.ModTime
		}
	}
	resp := LogFilesResponse{
		LogDir:    *common.LogDir,
		Enabled:   true,
		FileCount: len(files),
		TotalSize: totalSize,
		Files:     files,
	}
	if len(files) > 0 {
		resp.OldestTime = &oldest
		resp.NewestTime = &newest
	}
	common.ApiSuccess(c, resp)
}

// CleanupLogFiles 清理过期日志文件
func CleanupLogFiles(c *gin.Context) {
	mode := c.Query("mode")
	valueStr := c.Query("value")
	if mode != "by_count" && mode != "by_days" {
		common.ApiErrorMsg(c, "invalid mode, must be by_count or by_days")
		return
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil || value < 1 {
		common.ApiErrorMsg(c, "invalid value, must be a positive integer")
		return
	}
	if *common.LogDir == "" {
		common.ApiErrorMsg(c, "log directory not configured")
		return
	}

	files, err := getLogFiles()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	activeLogPath := logger.GetCurrentLogPath()
	var toDelete []LogFileInfo

	switch mode {
	case "by_count":
		// files 已按名称降序（最新在前），保留前 value 个
		for i, f := range files {
			if i < value {
				continue
			}
			fullPath := filepath.Join(*common.LogDir, f.Name)
			if fullPath == activeLogPath {
				continue
			}
			toDelete = append(toDelete, f)
		}
	case "by_days":
		cutoff := time.Now().AddDate(0, 0, -value)
		for _, f := range files {
			if f.ModTime.Before(cutoff) {
				fullPath := filepath.Join(*common.LogDir, f.Name)
				if fullPath == activeLogPath {
					continue
				}
				toDelete = append(toDelete, f)
			}
		}
	}

	var deletedCount int
	var freedBytes int64
	var failedFiles []string
	for _, f := range toDelete {
		fullPath := filepath.Join(*common.LogDir, f.Name)
		if err := os.Remove(fullPath); err != nil {
			failedFiles = append(failedFiles, f.Name)
			continue
		}
		deletedCount++
		freedBytes += f.Size
	}

	result := gin.H{
		"deleted_count": deletedCount,
		"freed_bytes":   freedBytes,
		"failed_files":  failedFiles,
	}

	if len(failedFiles) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("部分文件删除失败（%d/%d）", len(failedFiles), len(toDelete)),
			"data":    result,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// getDiskCacheInfo 获取磁盘缓存目录信息
func getDiskCacheInfo() DiskCacheInfo {
	// 使用统一的缓存目录
	dir := common.GetDiskCacheDir()

	info := DiskCacheInfo{
		Path:   dir,
		Exists: false,
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return info
	}

	info.Exists = true
	info.FileCount = 0
	info.TotalSize = 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info.FileCount++
		if fileInfo, err := entry.Info(); err == nil {
			info.TotalSize += fileInfo.Size()
		}
	}

	return info
}

// GetDashboardHealthData returns high-level health data for the dashboard.
func GetDashboardHealthData(c *gin.Context) {
	status := common.GetSystemStatus()
	monitorCfg := common.GetPerformanceMonitorConfig()
	rlCfg := middleware.GetRequestQueueConfig()

	// Get channel counts.
	channelCount := 0
	enabledCount := 0
	openBreakers := 0
	if channels := model.GetAllEnabledChannels(); channels != nil {
		enabledCount = len(channels)
	}
	if total, err := model.CountAllChannels(); err == nil {
		channelCount = int(total)
	}
	openBreakers = service.GetOpenCircuitBreakerCount()

	// Get in-flight request data.
	inFlight := model.GetAllInFlightRequests()

	data := gin.H{
		"system": gin.H{
			"cpu_usage":       status.CPUUsage,
			"memory_usage":    status.MemoryUsage,
			"disk_usage":      status.DiskUsage,
			"monitor_enabled": monitorCfg.Enabled,
		},
		"channels": gin.H{
			"total":           channelCount,
			"enabled":         enabledCount,
			"open_breakers":   openBreakers,
			"in_flight_total": sumInFlight(inFlight),
		},
		"request_queue": gin.H{
			"enabled":          rlCfg.Enabled,
			"active_requests":  middleware.GetActiveRequestCount(),
			"queue_size":       0,
			"max_concurrency":  rlCfg.MaxConcurrency,
		},
		"adaptive_limit": gin.H{
			"enabled":        middleware.GetAdaptiveRateLimitConfig().Enabled,
			"current_limit":  middleware.GetCurrentDynamicLimit(),
		},
	}

	if rlCfg.Enabled {
		stats := middleware.GetQueueStats()
		data["request_queue"].(gin.H)["queue_size"] = stats["queue_size"]
		data["request_queue"].(gin.H)["avg_wait_ms"] = stats["avg_wait_ms"]
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// GetDashboardChannelHealth returns channel health check results.
func GetDashboardChannelHealth(c *gin.Context) {
	results := service.GetAllChannelHealthResults()
	type healthItem struct {
		ChannelID           int    `json:"channel_id"`
		ChannelName         string `json:"channel_name"`
		LastChecked         string `json:"last_checked"`
		Success             bool   `json:"success"`
		LatencyMs           int64  `json:"latency_ms"`
		ErrorMsg            string `json:"error_msg,omitempty"`
		ConsecutiveFailures int    `json:"consecutive_failures"`
	}
	items := make([]healthItem, 0, len(results))
	for _, r := range results {
		items = append(items, healthItem{
			ChannelID:           r.ChannelID,
			ChannelName:         r.ChannelName,
			LastChecked:         r.LastChecked.Format(time.RFC3339),
			Success:             r.Success,
			LatencyMs:           r.LatencyMs,
			ErrorMsg:            r.ErrorMsg,
			ConsecutiveFailures: r.ConsecutiveFailures,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// GetDashboardCircuitBreakers returns the state of all circuit breakers.
func GetDashboardCircuitBreakers(c *gin.Context) {
	states := service.GetAllCircuitBreakerStates()
	type breakerItem struct {
		ChannelID int    `json:"channel_id"`
		State     string `json:"state"`
	}
	items := make([]breakerItem, 0, len(states))
	for chID, state := range states {
		stateStr := "closed"
		switch state {
		case service.CircuitBreakerOpen:
			stateStr = "open"
		case service.CircuitBreakerHalfOpen:
			stateStr = "half_open"
		}
		items = append(items, breakerItem{ChannelID: chID, State: stateStr})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// GetDashboardInFlight returns in-flight request counts per channel.
func GetDashboardInFlight(c *gin.Context) {
	inFlight := model.GetAllInFlightRequests()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    inFlight,
	})
}

func sumInFlight(m map[int]int64) int64 {
	var total int64
	for _, v := range m {
		total += v
	}
	return total
}
