package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	modelRateLimitTimeFormat              = "2006-01-02T15:04:05.000Z"
)

const redisModelRateLimitReserveScript = `
local key = KEYS[1]
local maxCount = tonumber(ARGV[1])
local duration = tonumber(ARGV[2])
local token = ARGV[3]
local nowReply = redis.call('TIME')
local now = tonumber(nowReply[1])
local length = redis.call('LLEN', key)
if length >= maxCount then
  local oldest = redis.call('LINDEX', key, -1)
  local separator = string.find(oldest, '|', 1, true)
  local oldestTime
  if separator then
    oldestTime = tonumber(string.sub(oldest, separator + 1))
  else
    oldestTime = tonumber(oldest)
  end
  if oldestTime then
    if now - oldestTime < duration then
      local ttl = redis.call('TTL', key)
      if ttl < 0 then redis.call('EXPIRE', key, duration) end
      return 0
    end
  elseif redis.call('TTL', key) > 0 then
    return 0
  end
end
redis.call('LPUSH', key, token .. '|' .. now)
redis.call('LTRIM', key, 0, maxCount - 1)
redis.call('EXPIRE', key, duration)
return 1
`

const redisModelRateLimitReleaseScript = `
local key = KEYS[1]
local prefix = ARGV[1] .. '|'
local entries = redis.call('LRANGE', key, 0, -1)
for _, entry in ipairs(entries) do
  if string.sub(entry, 1, string.len(prefix)) == prefix then
    local removed = redis.call('LREM', key, 1, entry)
    if removed > 0 and redis.call('LLEN', key) == 0 then redis.call('DEL', key) end
    return removed
  end
end
return 0
`

func reserveRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, string, error) {
	if maxCount == 0 {
		return true, "", nil
	}
	if maxCount < 0 {
		return false, "", fmt.Errorf("rate limit maximum must not be negative")
	}
	if duration <= 0 {
		return false, "", fmt.Errorf("rate limit duration must be positive")
	}
	if rdb == nil {
		return false, "", fmt.Errorf("Redis client is not initialized")
	}
	token := uuid.NewString()
	result, err := rdb.Eval(ctx, redisModelRateLimitReserveScript, []string{key}, maxCount, duration, token).Int()
	if err != nil {
		return false, "", err
	}
	if result != 1 {
		return false, "", nil
	}
	return true, token, nil
}

func releaseRedisRateLimit(ctx context.Context, rdb *redis.Client, key, token string) error {
	if token == "" {
		return nil
	}
	if rdb == nil {
		return fmt.Errorf("Redis client is not initialized")
	}
	_, err := rdb.Eval(ctx, redisModelRateLimitReleaseScript, []string{key}, token).Int()
	return err
}

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(modelRateLimitTimeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().UTC().Format(modelRateLimitTimeFormat)
	nowTime, err := time.Parse(modelRateLimitTimeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().UTC().Format(modelRateLimitTimeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, userId)
		allowed, reservationToken, err := reserveRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			writeOpenAiRateLimited(c, duration, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}

		//2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s", userId)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*duration),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)

			if err != nil {
				_ = releaseRedisRateLimit(ctx, rdb, successKey, reservationToken)
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				_ = releaseRedisRateLimit(ctx, rdb, successKey, reservationToken)
				writeOpenAiRateLimited(c, duration, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
				return
			}
		}

		// 4. 处理请求
		c.Next()

		// 5. 失败请求释放成功名额
		if c.Writer.Status() >= 400 {
			if err := releaseRedisRateLimit(ctx, rdb, successKey, reservationToken); err != nil {
				fmt.Println("释放成功请求数限制失败:", err.Error())
			}
		}
	}
}
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + userId
		successKey := ModelRequestRateLimitSuccessCountMark + userId

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			writeOpenAiRateLimited(c, duration, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
			return
		}

		// 2. 检查成功请求数限制
		// 使用一个临时key来检查限制，这样可以避免实际记录
		reservation, allowed := inMemoryRateLimiter.Reserve(successKey, successMaxCount, duration)
		if !allowed {
			writeOpenAiRateLimited(c, duration, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 只有成功请求才保留成功名额。
		if c.Writer.Status() < 400 {
			reservation.Commit()
		} else {
			reservation.Rollback()
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyUserGroup)

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		}
	}
}
