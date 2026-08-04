package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// InMemoryRateLimiter: sharded concurrency tests
// ---------------------------------------------------------------------------

func TestInMemoryRateLimiterShardedAllowsExactlyMaxPerKey(t *testing.T) {
	var limiter common.InMemoryRateLimiter
	limiter.Init(0)

	const (
		goroutines = 50
		maxReq     = 5
	)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if limiter.Request("shared-key", maxReq, 10) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(maxReq), allowed.Load())
}

func TestInMemoryRateLimiterShardedIsolatesKeys(t *testing.T) {
	var limiter common.InMemoryRateLimiter
	limiter.Init(0)

	const keys = 128
	var wg sync.WaitGroup
	wg.Add(keys)
	for i := range keys {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				limiter.Request("key-"+strconv.Itoa(i), 10, 60)
			}
		}()
	}
	wg.Wait()

	// Verify all keys are isolated by checking reserves.
	for i := range keys {
		_, ok := limiter.Reserve("key-"+strconv.Itoa(i), 10, 60)
		assert.True(t, ok, "key-%d should still have available slots", i)
	}
}

func TestInMemoryRateLimiterReserveCommitRollback(t *testing.T) {
	var limiter common.InMemoryRateLimiter
	limiter.Init(0)

	// Reserve a slot.
	res, ok := limiter.Reserve("test-key", 1, 30)
	require.True(t, ok)
	require.NotNil(t, res)

	// Second reservation should fail.
	_, ok = limiter.Reserve("test-key", 1, 30)
	assert.False(t, ok)

	// Rollback and retry.
	res.Rollback()
	res2, ok := limiter.Reserve("test-key", 1, 30)
	require.True(t, ok)
	res2.Commit()
}

func TestInMemoryRateLimiterExpiration(t *testing.T) {
	var limiter common.InMemoryRateLimiter
	limiter.Init(100 * time.Millisecond)

	limiter.Request("exp-key", 1, 1)
	_, ok := limiter.Reserve("exp-key", 1, 1)
	assert.False(t, ok, "should be rate-limited immediately")

	time.Sleep(1100 * time.Millisecond) // 1s window + slop
	_, ok = limiter.Reserve("exp-key", 1, 1)
	assert.True(t, ok, "should be allowed after window expires")
}

func TestInMemoryRateLimiterZeroMaxDisables(t *testing.T) {
	var limiter common.InMemoryRateLimiter
	limiter.Init(0)

	res, ok := limiter.Reserve("zero-key", 0, 10)
	assert.True(t, ok)
	assert.Equal(t, common.RateLimitReservationNoop, res.State())
}

// ---------------------------------------------------------------------------
// Request Queue tests
// ---------------------------------------------------------------------------

func TestRequestQueueEnforcesMaxConcurrency(t *testing.T) {
	SetRequestQueueConfig(RequestQueueConfig{
		Enabled:        true,
		MaxQueueSize:   100,
		MaxConcurrency: 2,
		QueueTimeout:   100 * time.Millisecond,
	})
	defer SetRequestQueueConfig(RequestQueueConfig{})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestQueueMiddleware())
	router.GET("/slow", func(c *gin.Context) {
		time.Sleep(200 * time.Millisecond)
		c.Status(http.StatusNoContent)
	})

	// Start 3 concurrent requests; only 2 should run, 1 should time out.
	results := make(chan int, 3)
	for range 3 {
		go func() {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/slow", nil))
			results <- w.Code
		}()
	}

	timeouts := 0
	successes := 0
	for range 3 {
		code := <-results
		if code == http.StatusServiceUnavailable {
			timeouts++
		} else {
			successes++
		}
	}
	assert.Equal(t, 2, successes, "only 2 requests should succeed")
	assert.Equal(t, 1, timeouts, "1 request should time out")
}

// ---------------------------------------------------------------------------
// Priority Middleware tests
// ---------------------------------------------------------------------------

func TestPriorityMiddlewareResolvesAdmin(t *testing.T) {
	SetPriorityConfig(PriorityConfig{Enabled: true})
	defer SetPriorityConfig(PriorityConfig{})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("role", 10) // admin
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		c.Next()
	})
	router.Use(PriorityMiddleware())
	router.GET("/test", func(c *gin.Context) {
		prio := common.GetContextKeyString(c, constant.ContextKeyRequestPriority)
		c.String(http.StatusOK, prio)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))
	assert.Equal(t, "admin", w.Body.String())
}

func TestPriorityMiddlewareResolvesFreeUser(t *testing.T) {
	SetPriorityConfig(PriorityConfig{Enabled: true})
	defer SetPriorityConfig(PriorityConfig{})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 2)
		c.Set("role", 0) // regular user
		common.SetContextKey(c, constant.ContextKeyUserGroup, "unknown-group")
		c.Next()
	})
	router.Use(PriorityMiddleware())
	router.GET("/test", func(c *gin.Context) {
		prio := common.GetContextKeyString(c, constant.ContextKeyRequestPriority)
		c.String(http.StatusOK, prio)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))
	assert.Equal(t, "free", w.Body.String())
}

// ---------------------------------------------------------------------------
// Tracing Middleware tests
// ---------------------------------------------------------------------------

func TestTracingMiddlewareGeneratesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TracingMiddleware())
	router.GET("/test", func(c *gin.Context) {
		traceID := GetTraceID(c)
		assert.NotEmpty(t, traceID)
		c.Header("X-Trace-Id", traceID)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))
	assert.NotEmpty(t, w.Header().Get("X-Trace-Id"))
	assert.NotEmpty(t, w.Header().Get("X-Trace-Duration-Ms"))
}

func TestTracingMiddlewarePropagatesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TracingMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Trace-Id", "client-trace-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, "client-trace-123", w.Header().Get("X-Trace-Id"))
}

// ---------------------------------------------------------------------------
// Adaptive Rate Limiting tests
// ---------------------------------------------------------------------------

func TestAdaptiveRateLimiterConfig(t *testing.T) {
	cfg := AdaptiveRateLimitConfig{
		Enabled:             true,
		BaseLimit:           100,
		MaxLimit:            500,
		MinLimit:            10,
		CPUHighThreshold:    80,
		MemoryHighThreshold: 80,
		AdjustIntervalSec:   10,
		Aggressiveness:      0.5,
	}
	SetAdaptiveRateLimitConfig(cfg)

	got := GetAdaptiveRateLimitConfig()
	assert.True(t, got.Enabled)
	assert.Equal(t, 100, got.BaseLimit)
	assert.Equal(t, 500, got.MaxLimit)
	assert.Equal(t, 10, got.MinLimit)
}

func TestAdaptiveRateLimiterDynamicLimitBounds(t *testing.T) {
	SetAdaptiveRateLimitConfig(AdaptiveRateLimitConfig{
		Enabled:             true,
		BaseLimit:           100,
		MaxLimit:            500,
		MinLimit:            10,
		CPUHighThreshold:    80,
		MemoryHighThreshold: 80,
		AdjustIntervalSec:   1,
		Aggressiveness:      0.5,
	})
	defer SetAdaptiveRateLimitConfig(AdaptiveRateLimitConfig{})

	limit := GetCurrentDynamicLimit()
	assert.GreaterOrEqual(t, limit, int32(10))
	assert.LessOrEqual(t, limit, int32(500))
}

// ---------------------------------------------------------------------------
// Performance Middleware tests
// ---------------------------------------------------------------------------

func TestSystemPerformanceCheckPassesWhenDisabled(t *testing.T) {
	prev := common.GetPerformanceMonitorConfig()
	common.SetPerformanceMonitorConfig(common.PerformanceMonitorConfig{Enabled: false})
	defer common.SetPerformanceMonitorConfig(prev)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SystemPerformanceCheck())
	router.GET("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// ---------------------------------------------------------------------------
// RateLimitReservation State() helper check
// ---------------------------------------------------------------------------

func TestRateLimitReservationStates(t *testing.T) {
	var limiter common.InMemoryRateLimiter
	limiter.Init(0)

	// Noop reservation for zero max.
	res, ok := limiter.Reserve("state-key", 0, 10)
	assert.True(t, ok)
	assert.Equal(t, common.RateLimitReservationNoop, res.State())

	// Pending reservation.
	res, ok = limiter.Reserve("state-key", 1, 10)
	require.True(t, ok)
	assert.Equal(t, common.RateLimitReservationPending, res.State())

	// After commit.
	res.Commit()
	assert.Equal(t, common.RateLimitReservationCommitted, res.State())

	// Rollback after commit has no effect.
	res.Rollback()
	assert.Equal(t, common.RateLimitReservationCommitted, res.State())
}