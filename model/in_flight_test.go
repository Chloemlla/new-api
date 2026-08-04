package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// In-flight request tracking tests
// ---------------------------------------------------------------------------

func TestIncrementInFlightRequests(t *testing.T) {
	inFlightRequests = sync.Map{}

	IncrementInFlightRequests(1)
	assert.Equal(t, int64(1), GetInFlightRequests(1))

	IncrementInFlightRequests(1)
	assert.Equal(t, int64(2), GetInFlightRequests(1))
}

func TestDecrementInFlightRequests(t *testing.T) {
	inFlightRequests = sync.Map{}

	IncrementInFlightRequests(10)
	IncrementInFlightRequests(10)
	DecrementInFlightRequests(10)
	assert.Equal(t, int64(1), GetInFlightRequests(10))

	DecrementInFlightRequests(10)
	assert.Equal(t, int64(0), GetInFlightRequests(10))

	assert.NotPanics(t, func() { DecrementInFlightRequests(10) })
}

func TestInFlightNonExistentChannel(t *testing.T) {
	inFlightRequests = sync.Map{}
	assert.Equal(t, int64(0), GetInFlightRequests(99999))
	assert.NotPanics(t, func() { DecrementInFlightRequests(99999) })
}

func TestGetAllInFlightRequestsEmpty(t *testing.T) {
	inFlightRequests = sync.Map{}
	all := GetAllInFlightRequests()
	assert.Empty(t, all)
}

func TestConcurrentInFlightRequests(t *testing.T) {
	inFlightRequests = sync.Map{}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			IncrementInFlightRequests(1)
			IncrementInFlightRequests(2)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(100), GetInFlightRequests(1))
	assert.Equal(t, int64(100), GetInFlightRequests(2))
}

// ---------------------------------------------------------------------------
// Load-aware channel selection (no DB required)
// ---------------------------------------------------------------------------

func TestGetRandomSatisfiedChannelWithLoadAwareNoPanic(t *testing.T) {
	// When memory cache is disabled, the function calls GetChannel() which
	// requires a database. Just verify it doesn't panic when called with
	// empty cache.
	prevMemCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	defer func() { common.MemoryCacheEnabled = prevMemCache }()

	assert.NotPanics(t, func() {
		_, _ = GetRandomSatisfiedChannelWithLoadAware("nonexistent", "nonexistent-model", 0, "")
	})
}

// ---------------------------------------------------------------------------
// GetAllEnabledChannels
// ---------------------------------------------------------------------------

func TestGetAllEnabledChannelsReturnsEmptyWhenNoCache(t *testing.T) {
	prevMemCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	defer func() { common.MemoryCacheEnabled = prevMemCache }()

	channels := GetAllEnabledChannels()
	// Should not panic; returns whatever DB returns (or empty).
	assert.NotNil(t, channels)
}