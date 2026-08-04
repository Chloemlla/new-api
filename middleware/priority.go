package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

// RequestPriority levels.
const (
	ReqPriorityUserFree    = "free"
	ReqPriorityUserBasic   = "basic"
	ReqPriorityUserPremium = "premium"
	ReqPriorityAdmin       = "admin"
)

// PriorityConfig holds the configuration for request priority handling.
type PriorityConfig struct {
	Enabled bool
}

var priorityConfig PriorityConfig

// GetPriorityConfig returns the current priority configuration.
func GetPriorityConfig() PriorityConfig {
	return priorityConfig
}

// SetPriorityConfig updates the priority configuration.
func SetPriorityConfig(cfg PriorityConfig) {
	priorityConfig = cfg
}

// PriorityMiddleware returns a Gin middleware that marks requests with their
// priority level based on the authenticated user's role and group.
// It must be placed AFTER authentication middleware.
func PriorityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := GetPriorityConfig()
		if !cfg.Enabled {
			c.Next()
			return
		}

		priority := resolveRequestPriority(c)
		common.SetContextKey(c, constant.ContextKeyRequestPriority, priority)
		c.Set("request_priority", priority)

		c.Next()
	}
}

// resolveRequestPriority determines the priority level for the current request.
func resolveRequestPriority(c *gin.Context) string {
	// Check if the request is from an admin.
	role := c.GetInt("role")
	if role >= 10 { // Admin or root
		return ReqPriorityAdmin
	}

	// Check user group.
	group := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if group == "" {
		group = c.GetString("group")
	}

	// Premium groups.
	premiumGroups := common.GetEnvOrDefaultString("PREMIUM_GROUPS", "vip,premium,pro")
	if strings.Contains(premiumGroups, group) {
		return ReqPriorityUserPremium
	}

	// Basic groups.
	basicGroups := common.GetEnvOrDefaultString("BASIC_GROUPS", "basic,default")
	if strings.Contains(basicGroups, group) {
		return ReqPriorityUserBasic
	}

	return ReqPriorityUserFree
}

// ShouldDegradeRequest checks if a request should be degraded based on
// system load and the request's priority level. Returns true if the request
// should be rejected or handled with reduced quality.
func ShouldDegradeRequest(c *gin.Context) bool {
	cfg := GetPriorityConfig()
	if !cfg.Enabled {
		return false
	}

	// Only degrade free and basic users.
	priority := common.GetContextKeyString(c, constant.ContextKeyRequestPriority)
	if priority == ReqPriorityAdmin || priority == ReqPriorityUserPremium {
		return false
	}

	// Check system load.
	status := common.GetSystemStatus()
	monitorCfg := common.GetPerformanceMonitorConfig()
	if !monitorCfg.Enabled {
		return false
	}

	// Degrade when CPU or memory is above 80% of the configured threshold.
	effectiveCPUThreshold := float64(monitorCfg.CPUThreshold) * 0.8
	effectiveMemThreshold := float64(monitorCfg.MemoryThreshold) * 0.8

	if monitorCfg.CPUThreshold > 0 && status.CPUUsage > effectiveCPUThreshold {
		return true
	}
	if monitorCfg.MemoryThreshold > 0 && status.MemoryUsage > effectiveMemThreshold {
		return true
	}

	return false
}

// DegradeMiddleware returns a Gin middleware that rejects low-priority requests
// when the system is under heavy load. Must be placed BEFORE the relay handler.
func DegradeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if ShouldDegradeRequest(c) {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"message": "System is under heavy load. Please try again later.",
					"type":    "server_overloaded",
					"code":    "server_overloaded",
				},
			})
			return
		}
		c.Next()
	}
}