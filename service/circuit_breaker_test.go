package service

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Circuit Breaker tests
// ---------------------------------------------------------------------------

func TestCircuitBreakerStartsClosed(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{Enabled: true})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	assert.False(t, IsCircuitBreakerOpen(1), "a new circuit breaker should be closed")
}

func TestCircuitBreakerOpensAfterFailures(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 2,
		TimeoutDuration:  60 * time.Second,
		HalfOpenMaxRequests: 1,
	})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	for i := 0; i < 3; i++ {
		RecordCircuitBreakerFailure(42)
	}
	assert.True(t, IsCircuitBreakerOpen(42), "should be open after 3 failures")
}

func TestCircuitBreakerClosesAfterSuccessesInHalfOpen(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{
		Enabled:            true,
		FailureThreshold:   2,
		SuccessThreshold:   2,
		TimeoutDuration:    50 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	RecordCircuitBreakerFailure(99)
	RecordCircuitBreakerFailure(99)
	require.True(t, IsCircuitBreakerOpen(99))

	time.Sleep(60 * time.Millisecond)

	halfOpen := IsCircuitBreakerOpen(99)
	assert.False(t, halfOpen, "should be half-open after timeout")

	RecordCircuitBreakerSuccess(99)
	RecordCircuitBreakerSuccess(99)

	state, ok := GetCircuitBreakerState(99)
	require.True(t, ok)
	assert.Equal(t, CircuitBreakerClosed, state, "should be closed after 2 half-open successes")
}

func TestCircuitBreakerReopensOnHalfOpenFailure(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{
		Enabled:            true,
		FailureThreshold:   2,
		SuccessThreshold:   3,
		TimeoutDuration:    50 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	RecordCircuitBreakerFailure(100)
	RecordCircuitBreakerFailure(100)
	require.True(t, IsCircuitBreakerOpen(100))

	time.Sleep(60 * time.Millisecond)

	RecordCircuitBreakerFailure(100)
	state, ok := GetCircuitBreakerState(100)
	require.True(t, ok)
	assert.Equal(t, CircuitBreakerOpen, state, "should re-open on half-open failure")
}

func TestCircuitBreakerReset(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		TimeoutDuration:  60 * time.Second,
	})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	RecordCircuitBreakerFailure(200)
	require.True(t, IsCircuitBreakerOpen(200))

	ResetCircuitBreaker(200)
	state, ok := GetCircuitBreakerState(200)
	require.True(t, ok)
	assert.Equal(t, CircuitBreakerClosed, state, "should be closed after manual reset")
}

func TestCircuitBreakerConcurrentSafety(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 5,
		SuccessThreshold: 3,
		TimeoutDuration:  100 * time.Millisecond,
		HalfOpenMaxRequests: 10,
	})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RecordCircuitBreakerFailure(300)
			RecordCircuitBreakerSuccess(300)
			IsCircuitBreakerOpen(300)
		}()
	}
	wg.Wait()
	assert.NotPanics(t, func() { GetAllCircuitBreakerStates() })
}

func TestCircuitBreakerDisabledWhenConfigOff(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{Enabled: false})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	for i := 0; i < 100; i++ {
		RecordCircuitBreakerFailure(400)
	}
	assert.False(t, IsCircuitBreakerOpen(400), "should not open when disabled")
}

func TestGetOpenCircuitBreakerCount(t *testing.T) {
	SetCircuitBreakerConfig(CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		TimeoutDuration:  60 * time.Second,
	})
	defer SetCircuitBreakerConfig(CircuitBreakerConfig{})

	RecordCircuitBreakerFailure(10)
	RecordCircuitBreakerFailure(11)
	count := GetOpenCircuitBreakerCount()
	assert.GreaterOrEqual(t, count, 2)
}

// ---------------------------------------------------------------------------
// Channel Health Probe tests
// ---------------------------------------------------------------------------

func TestChannelHealthProbeConfig(t *testing.T) {
	SetChannelHealthProbeConfig(ChannelHealthProbeConfig{
		Enabled:         true,
		IntervalSeconds: 30,
		TimeoutSeconds:  5,
		Concurrency:     5,
	})

	cfg := GetChannelHealthProbeConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 30, cfg.IntervalSeconds)
	assert.Equal(t, 5, cfg.TimeoutSeconds)
	assert.Equal(t, 5, cfg.Concurrency)

	SetChannelHealthProbeConfig(ChannelHealthProbeConfig{})
}

func TestChannelHealthResultConsecutiveFailures(t *testing.T) {
	channelHealthResults = sync.Map{}

	channelID := 500
	result := &ChannelHealthResult{
		ChannelID:   channelID,
		ChannelName: "test-channel",
		Success:     false,
		ErrorMsg:    "test error",
	}

	for i := 1; i <= 3; i++ {
		finalizeHealthResult(channelID, result)
		r := GetChannelHealthResult(channelID)
		require.NotNil(t, r)
		assert.Equal(t, i, r.ConsecutiveFailures, "failure %d", i)
	}

	successResult := &ChannelHealthResult{
		ChannelID:   channelID,
		ChannelName: "test-channel",
		Success:     true,
	}
	finalizeHealthResult(channelID, successResult)
	r := GetChannelHealthResult(channelID)
	require.NotNil(t, r)
	assert.Equal(t, 0, r.ConsecutiveFailures, "success should reset counter")
}

func TestBuildHealthCheckURL(t *testing.T) {
	ch := &model.Channel{
		Id:   1,
		Type: 1,
	}
	url := buildHealthCheckURL(ch, "https://api.openai.com")
	assert.Equal(t, "https://api.openai.com/v1/models", url)

	ch2 := &model.Channel{
		Id:   2,
		Type: 60,
	}
	url2 := buildHealthCheckURL(ch2, "https://custom.example.com")
	assert.Empty(t, url2)
}

func TestGetUnhealthyChannels(t *testing.T) {
	channelHealthResults = sync.Map{}

	channelHealthResults.Store(1, &ChannelHealthResult{ChannelID: 1, Success: true})
	channelHealthResults.Store(2, &ChannelHealthResult{ChannelID: 2, Success: false, ErrorMsg: "error"})
	channelHealthResults.Store(3, &ChannelHealthResult{ChannelID: 3, Success: true})

	unhealthy := GetUnhealthyChannels()
	assert.Len(t, unhealthy, 1)
	assert.Equal(t, 2, unhealthy[0].ChannelID)
}

// ---------------------------------------------------------------------------
// Traffic Mirroring tests
// ---------------------------------------------------------------------------

func TestGetChannelMirrorConfig(t *testing.T) {
	SetChannelMirrorConfig(ChannelMirrorConfig{
		Enabled:     true,
		MirrorRatio: 0.5,
	})

	cfg := GetChannelMirrorConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 0.5, cfg.MirrorRatio)

	SetChannelMirrorConfig(ChannelMirrorConfig{})
}

func TestMirrorDisabledWhenRatioZero(t *testing.T) {
	SetChannelMirrorConfig(ChannelMirrorConfig{
		Enabled:     true,
		MirrorRatio: 0.0,
	})
	defer SetChannelMirrorConfig(ChannelMirrorConfig{})

	assert.NotPanics(t, func() {
		MaybeMirrorRequest(nil, nil, nil)
	})
}

// ---------------------------------------------------------------------------
// Combined in-flight + circuit breaker integration helpers
// ---------------------------------------------------------------------------

func TestInFlightAndCircuitBreakerDoNotPanic(t *testing.T) {
	// Verify that calling the public API functions doesn't panic.
	assert.NotPanics(t, func() {
		model.IncrementInFlightRequests(999)
		model.DecrementInFlightRequests(999)
		_ = model.GetInFlightRequests(999)
		_ = model.GetAllInFlightRequests()
	})
}

// ---------------------------------------------------------------------------
// common env helper test
// ---------------------------------------------------------------------------

func TestGetEnvOrDefaultDuration(t *testing.T) {
	d := common.GetEnvOrDefaultDuration("UNSET_ENV_FOR_TEST", 30)
	assert.Equal(t, 30*time.Second, d)
}