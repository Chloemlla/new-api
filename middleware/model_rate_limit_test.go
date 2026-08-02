package middleware

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func modelRateLimitTestRouter(userID int, handler gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.GET(
		"/limited",
		func(c *gin.Context) {
			c.Set("id", userID)
			c.Set(common.RequestIdKey, "test-request")
		},
		handler,
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	return router
}

func TestModelRedisRateLimitUsesOpenAIResponseAndRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, _ = useRateLimitMiniRedis(t)

	previousDuration := setting.ModelRequestRateLimitDurationMinutes
	setting.ModelRequestRateLimitDurationMinutes = 1
	t.Cleanup(func() { setting.ModelRequestRateLimitDurationMinutes = previousDuration })

	router := modelRateLimitTestRouter(9301, redisRateLimitHandler(60, 0, 1))
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.70:12345").Code)

	response := performRateLimitRequest(router, "/limited", "192.0.2.70:12345")
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.Equal(t, "60", response.Header().Get("Retry-After"))
	assert.JSONEq(t, `{"error":{"message":"您已达到请求数限制：1分钟内最多请求1次 (request id: test-request)","type":"new_api_error","code":""}}`, response.Body.String())
}

func TestModelMemoryRateLimitUsesOpenAIResponseAndRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousDuration := setting.ModelRequestRateLimitDurationMinutes
	setting.ModelRequestRateLimitDurationMinutes = 1
	t.Cleanup(func() { setting.ModelRequestRateLimitDurationMinutes = previousDuration })

	router := modelRateLimitTestRouter(9302, memoryRateLimitHandler(60, 0, 1))
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.71:12345").Code)

	response := performRateLimitRequest(router, "/limited", "192.0.2.71:12345")
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.Equal(t, "60", response.Header().Get("Retry-After"))
	assert.JSONEq(t, `{"error":{"message":"您已达到请求数限制：1分钟内最多请求1次 (request id: test-request)","type":"new_api_error","code":""}}`, response.Body.String())
}

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}
