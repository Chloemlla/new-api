package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRequestRateLimitUsesPersistedUserGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousEnabled := setting.ModelRequestRateLimitEnabled
	previousDuration := setting.ModelRequestRateLimitDurationMinutes
	previousTotal := setting.ModelRequestRateLimitCount
	previousSuccess := setting.ModelRequestRateLimitSuccessCount
	previousGroups := setting.ModelRequestRateLimitGroup
	previousRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled = previousEnabled
		setting.ModelRequestRateLimitDurationMinutes = previousDuration
		setting.ModelRequestRateLimitCount = previousTotal
		setting.ModelRequestRateLimitSuccessCount = previousSuccess
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = previousGroups
		setting.ModelRequestRateLimitMutex.Unlock()
		common.RedisEnabled = previousRedisEnabled
	})

	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 100
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{"default":[1,100],"vip":[0,100]}`))
	common.RedisEnabled = false

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 918273)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "vip")
		c.Next()
	})
	router.GET("/model", ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/model", nil))
	assert.Equal(t, http.StatusNoContent, first.Code)

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/model", nil))
	assert.Equal(t, http.StatusTooManyRequests, second.Code)
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

func TestRedisModelSuccessReservationIsAtomicUnderConcurrency(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	const (
		requestCount = 32
		maximumCount = 7
		duration     = int64(41)
	)
	key := "rateLimit:MRRLS:concurrent-user"

	var allowedCount atomic.Int64
	errorsFound := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for range requestCount {
		go func() {
			defer waitGroup.Done()
			allowed, token, err := reserveRedisRateLimit(context.Background(), redisClient, key, maximumCount, duration)
			if err != nil {
				errorsFound <- err
				return
			}
			if allowed {
				allowedCount.Add(1)
			} else {
				assert.Empty(t, token)
			}
		}()
	}
	waitGroup.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}

	assert.Equal(t, int64(maximumCount), allowedCount.Load())
	length, err := redisClient.LLen(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(maximumCount), length)
	assert.Equal(t, time.Duration(duration)*time.Second, redisServer.TTL(key))
}

func TestRedisModelSuccessReservationReleasesFailedRequest(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	ctx := context.Background()
	key := "rateLimit:MRRLS:release-user"

	allowed, token, err := reserveRedisRateLimit(ctx, redisClient, key, 1, 60)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NotEmpty(t, token)

	allowed, _, err = reserveRedisRateLimit(ctx, redisClient, key, 1, 60)
	require.NoError(t, err)
	assert.False(t, allowed)

	require.NoError(t, releaseRedisRateLimit(ctx, redisClient, key, token))
	allowed, _, err = reserveRedisRateLimit(ctx, redisClient, key, 1, 60)
	require.NoError(t, err)
	assert.True(t, allowed)
}
