package common

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterShardedAllowsExactlyMaxPerKey(t *testing.T) {
	var limiter InMemoryRateLimiter
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
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	const keys = 128
	var wg sync.WaitGroup
	wg.Add(keys)
	for i := range keys {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				limiter.Request(keyForInt(i), 10, 60)
			}
		}()
	}
	wg.Wait()

	// Each key should still have room (3 used out of 10 max).
	for i := range keys {
		_, ok := limiter.Reserve(keyForInt(i), 10, 60)
		assert.True(t, ok, "key-%d should still have available slots", i)
	}
}

func TestInMemoryRateLimiterReserveCommitRollback(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	res, ok := limiter.Reserve("test-key", 1, 30)
	require.True(t, ok)
	require.NotNil(t, res)
	assert.Equal(t, RateLimitReservationPending, res.State())

	_, ok = limiter.Reserve("test-key", 1, 30)
	assert.False(t, ok)

	res.Rollback()
	assert.Equal(t, RateLimitReservationRolledBack, res.State())

	res2, ok := limiter.Reserve("test-key", 1, 30)
	require.True(t, ok)
	res2.Commit()
	assert.Equal(t, RateLimitReservationCommitted, res2.State())
}

func TestInMemoryRateLimiterZeroMaxDisables(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	res, ok := limiter.Reserve("zero-key", 0, 10)
	assert.True(t, ok)
	assert.Equal(t, RateLimitReservationNoop, res.State())
}

func TestInMemoryRateLimiterExpiration(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(100 * time.Millisecond)

	limiter.Request("exp-key", 1, 1)
	_, ok := limiter.Reserve("exp-key", 1, 1)
	assert.False(t, ok)

	time.Sleep(1100 * time.Millisecond)
	_, ok = limiter.Reserve("exp-key", 1, 1)
	assert.True(t, ok)
}

func TestInMemoryRateLimiterCancel(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	limiter.Request("cancel-key", 5, 60)
	limiter.Cancel("cancel-key")

	res, ok := limiter.Reserve("cancel-key", 5, 60)
	require.True(t, ok)
	res.Commit()
}

func TestInMemoryRateLimiterConcurrentDifferentKeys(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				limiter.Request(keyForInt(i*100+j), 100, 60)
			}
		}()
	}
	wg.Wait()
	// No deadlock = success.
}

func TestInMemoryRateLimiterMultipleShards(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	// 128 keys spread across 64 shards.
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiter.Request(keyForInt(i), 3, 60)
			limiter.Request(keyForInt(i), 3, 60)
			limiter.Request(keyForInt(i), 3, 60)
		}()
	}
	wg.Wait()
}

func keyForInt(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}