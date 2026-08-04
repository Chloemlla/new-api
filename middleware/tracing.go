package middleware

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

const (
	// TracingEnabled is the context key for the tracing flag.
	TracingEnabled = "tracing_enabled"

	// Context keys for tracing.
	ContextKeyTraceID        = "trace_id"
	ContextKeyRequestStart   = "trace_request_start"
	ContextKeyAuthDuration   = "trace_auth_duration"
	ContextKeyRLDuration     = "trace_rate_limit_duration"
	ContextKeyDistDuration   = "trace_distribution_duration"
	ContextKeyRelayDuration  = "trace_relay_duration"
	ContextKeyBillingDuration = "trace_billing_duration"
	ContextKeyTotalDuration  = "trace_total_duration"
)

// TracingMiddleware creates a Gin middleware that adds distributed tracing
// support by generating trace IDs and tracking timing across the request lifecycle.
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate or propagate trace ID.
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = c.GetHeader("X-Request-Id")
		}
		if traceID == "" {
			traceID = fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), c.GetInt("id"))
		}

		common.SetContextKey(c, constant.ContextKey(ContextKeyTraceID), traceID)
		common.SetContextKey(c, constant.ContextKey(ContextKeyRequestStart), time.Now())

		// Set response headers.
		c.Header("X-Trace-Id", traceID)

		c.Next()

		// Record total duration.
		start := common.GetContextKeyTime(c, constant.ContextKey(ContextKeyRequestStart))
		if !start.IsZero() {
			total := time.Since(start)
			common.SetContextKey(c, constant.ContextKey(ContextKeyTotalDuration), total.Milliseconds())
			c.Header("X-Trace-Duration-Ms", fmt.Sprintf("%d", total.Milliseconds()))
		}
	}
}

// RecordTiming records a timing span in the context.
func RecordTiming(c *gin.Context, key string) {
	start := common.GetContextKeyTime(c, constant.ContextKey(ContextKeyRequestStart))
	if !start.IsZero() {
		duration := time.Since(start).Milliseconds()
		common.SetContextKey(c, constant.ContextKey(key), duration)
	}
}

// GetTraceTiming retrieves a timing span from the context.
func GetTraceTiming(c *gin.Context, key string) int64 {
	if v, ok := c.Get(key); ok {
		if ms, ok := v.(int64); ok {
			return ms
		}
	}
	return 0
}

// GetTraceID returns the trace ID from the context.
func GetTraceID(c *gin.Context) string {
	if v, ok := c.Get(ContextKeyTraceID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}