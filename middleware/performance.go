package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// AdaptiveRateLimitConfig holds configuration for adaptive rate limiting.
type AdaptiveRateLimitConfig struct {
	Enabled             bool
	BaseLimit           int     // Base requests per second
	MaxLimit            int     // Maximum requests per second
	MinLimit            int     // Minimum requests per second
	CPUHighThreshold    float64 // CPU% above which to reduce limit
	MemoryHighThreshold float64 // Memory% above which to reduce limit
	AdjustIntervalSec   int     // How often to re-calculate the limit
	Aggressiveness      float64 // 0.0-1.0, how aggressively to adjust
}

var (
	adaptiveRLConfig AdaptiveRateLimitConfig
	adaptiveRLMu     sync.RWMutex
	currentDynamicLimit int32
)

// GetAdaptiveRateLimitConfig returns the current adaptive rate limiting config.
func GetAdaptiveRateLimitConfig() AdaptiveRateLimitConfig {
	adaptiveRLMu.RLock()
	defer adaptiveRLMu.RUnlock()
	return adaptiveRLConfig
}

// SetAdaptiveRateLimitConfig updates the adaptive rate limiting config.
func SetAdaptiveRateLimitConfig(cfg AdaptiveRateLimitConfig) {
	adaptiveRLMu.Lock()
	adaptiveRLConfig = cfg
	adaptiveRLMu.Unlock()
	if cfg.Enabled {
		atomic.StoreInt32(&currentDynamicLimit, int32(cfg.BaseLimit))
		// Start the adjustment loop if not already running.
		startAdaptiveAdjuster.Do(func() {
			go adaptiveRateAdjuster()
		})
	}
}

var startAdaptiveAdjuster sync.Once

// GetCurrentDynamicLimit returns the current dynamically adjusted rate limit.
func GetCurrentDynamicLimit() int32 {
	return atomic.LoadInt32(&currentDynamicLimit)
}

// adaptiveRateAdjuster is a background goroutine that periodically adjusts
// the rate limit based on system load.
func adaptiveRateAdjuster() {
	for {
		cfg := GetAdaptiveRateLimitConfig()
		if !cfg.Enabled {
			time.Sleep(30 * time.Second)
			continue
		}

		status := common.GetSystemStatus()
		currentLimit := float64(atomic.LoadInt32(&currentDynamicLimit))

		// Calculate load factor based on the highest resource usage.
		cpuLoad := status.CPUUsage / cfg.CPUHighThreshold
		memLoad := status.MemoryUsage / cfg.MemoryHighThreshold
		loadFactor := math.Max(cpuLoad, memLoad)

		// Adjust the limit.
		var newLimit float64
		if loadFactor > 1.0 {
			// Overloaded: reduce limit.
			reduction := (loadFactor - 1.0) * cfg.Aggressiveness
			newLimit = currentLimit * (1.0 - reduction)
		} else if loadFactor < 0.7 {
			// Underloaded: gradually increase limit.
			increase := (1.0 - loadFactor) * cfg.Aggressiveness * 0.5
			newLimit = currentLimit * (1.0 + increase)
		} else {
			// Normal load: maintain current limit.
			time.Sleep(time.Duration(cfg.AdjustIntervalSec) * time.Second)
			continue
		}

		// Clamp to bounds.
		newLimit = math.Max(float64(cfg.MinLimit), math.Min(float64(cfg.MaxLimit), newLimit))
		atomic.StoreInt32(&currentDynamicLimit, int32(math.Round(newLimit)))

		common.SysLog(fmt.Sprintf("adaptive rate limit: adjusted to %.0f req/s (cpu=%.1f%%, mem=%.1f%%)",
			newLimit, status.CPUUsage, status.MemoryUsage))

		time.Sleep(time.Duration(cfg.AdjustIntervalSec) * time.Second)
	}
}

// SystemPerformanceCheck 检查系统性能中间件
func SystemPerformanceCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅检查 Relay 接口 (/v1, /v1beta 等)
		// 这里简单判断路径前缀，可以根据实际路由调整
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/v1/messages") {
			if err := checkSystemPerformance(); err != nil {
				c.JSON(err.StatusCode, gin.H{
					"error": err.ToClaudeError(),
				})
				c.Abort()
				return
			}
		} else {
			if err := checkSystemPerformance(); err != nil {
				c.JSON(err.StatusCode, gin.H{
					"error": err.ToOpenAIError(),
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// checkSystemPerformance 检查系统性能是否超过阈值
func checkSystemPerformance() *types.NewAPIError {
	config := common.GetPerformanceMonitorConfig()
	if !config.Enabled {
		return nil
	}

	status := common.GetSystemStatus()

	// 检查 CPU
	if config.CPUThreshold > 0 && int(status.CPUUsage) > config.CPUThreshold {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("system cpu overloaded (current: %.1f%%, threshold: %d%%)", status.CPUUsage, config.CPUThreshold),
			"system_cpu_overloaded", http.StatusServiceUnavailable)
	}

	// 检查内存
	if config.MemoryThreshold > 0 && int(status.MemoryUsage) > config.MemoryThreshold {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("system memory overloaded (current: %.1f%%, threshold: %d%%)", status.MemoryUsage, config.MemoryThreshold),
			"system_memory_overloaded", http.StatusServiceUnavailable)
	}

	// 检查磁盘
	if config.DiskThreshold > 0 && int(status.DiskUsage) > config.DiskThreshold {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("system disk overloaded (current: %.1f%%, threshold: %d%%)", status.DiskUsage, config.DiskThreshold),
			"system_disk_overloaded", http.StatusServiceUnavailable)
	}

	return nil
}