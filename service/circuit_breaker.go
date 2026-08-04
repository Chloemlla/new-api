package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
)

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState int32

const (
	CircuitBreakerClosed   CircuitBreakerState = 0 // Normal operation
	CircuitBreakerOpen     CircuitBreakerState = 1 // Rejecting requests
	CircuitBreakerHalfOpen CircuitBreakerState = 2 // Testing if service recovered
)

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	Enabled              bool
	FailureThreshold     int           // Number of failures to open the circuit
	SuccessThreshold     int           // Number of successes in half-open to close
	TimeoutDuration      time.Duration // Time to wait before moving to half-open
	HalfOpenMaxRequests  int           // Max requests allowed in half-open state
}

// channelCircuitBreaker tracks state for a single channel.
type channelCircuitBreaker struct {
	state              int32 // atomic: CircuitBreakerState
	failureCount       int32
	successCount       int32
	lastFailureTime    int64 // unix nano
	halfOpenAllowed    int32
	halfOpenUsed       int32
	config             CircuitBreakerConfig
}

var (
	circuitBreakers     sync.Map // map[int]*channelCircuitBreaker
	circuitBreakerConfig CircuitBreakerConfig
	circuitBreakerMu    sync.Mutex
)

// GetCircuitBreakerConfig returns the current circuit breaker configuration.
func GetCircuitBreakerConfig() CircuitBreakerConfig {
	circuitBreakerMu.Lock()
	defer circuitBreakerMu.Unlock()
	return circuitBreakerConfig
}

// SetCircuitBreakerConfig updates the circuit breaker configuration.
func SetCircuitBreakerConfig(cfg CircuitBreakerConfig) {
	circuitBreakerMu.Lock()
	circuitBreakerConfig = cfg
	circuitBreakerMu.Unlock()
}

// getOrCreateBreaker returns the circuit breaker for a channel, creating it if needed.
func getOrCreateBreaker(channelID int) *channelCircuitBreaker {
	cfg := GetCircuitBreakerConfig()
	cb, _ := circuitBreakers.LoadOrStore(channelID, &channelCircuitBreaker{
		config: cfg,
	})
	return cb.(*channelCircuitBreaker)
}

// IsCircuitBreakerOpen checks if the circuit breaker for a channel is open.
// Returns true if the request should be rejected.
func IsCircuitBreakerOpen(channelID int) bool {
	cfg := GetCircuitBreakerConfig()
	if !cfg.Enabled {
		return false
	}

	raw, ok := circuitBreakers.Load(channelID)
	if !ok {
		return false
	}
	cb := raw.(*channelCircuitBreaker)

	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	switch state {
	case CircuitBreakerClosed:
		return false
	case CircuitBreakerOpen:
		// Check if it's time to move to half-open.
		lastFailure := atomic.LoadInt64(&cb.lastFailureTime)
		if time.Since(time.Unix(0, lastFailure)) > cfg.TimeoutDuration {
			// Transition to half-open.
			if atomic.CompareAndSwapInt32(&cb.state, int32(CircuitBreakerOpen), int32(CircuitBreakerHalfOpen)) {
				atomic.StoreInt32(&cb.halfOpenAllowed, int32(cfg.HalfOpenMaxRequests))
				atomic.StoreInt32(&cb.halfOpenUsed, 0)
				atomic.StoreInt32(&cb.successCount, 0)
				logger.LogDebug(nil, "circuit breaker: channel #%d moved to half-open", channelID)
			}
			// Re-check state after the CAS (may have been changed by another goroutine).
			return CircuitBreakerState(atomic.LoadInt32(&cb.state)) != CircuitBreakerHalfOpen
		}
		return true
	case CircuitBreakerHalfOpen:
		used := atomic.AddInt32(&cb.halfOpenUsed, 1)
		if used > atomic.LoadInt32(&cb.halfOpenAllowed) {
			return true // Exceeded half-open request limit
		}
		return false
	default:
		return false
	}
}

// RecordCircuitBreakerSuccess records a successful request for the circuit breaker.
func RecordCircuitBreakerSuccess(channelID int) {
	cfg := GetCircuitBreakerConfig()
	if !cfg.Enabled {
		return
	}

	cb := getOrCreateBreaker(channelID)

	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	switch state {
	case CircuitBreakerClosed:
		atomic.StoreInt32(&cb.failureCount, 0)
	case CircuitBreakerHalfOpen:
		count := atomic.AddInt32(&cb.successCount, 1)
		if count >= int32(cfg.SuccessThreshold) {
			if atomic.CompareAndSwapInt32(&cb.state, int32(CircuitBreakerHalfOpen), int32(CircuitBreakerClosed)) {
				atomic.StoreInt32(&cb.failureCount, 0)
				atomic.StoreInt32(&cb.successCount, 0)
				logger.LogInfo(nil, fmt.Sprintf("circuit breaker: channel #%d closed (recovered)", channelID))
			}
		}
	}
}

// RecordCircuitBreakerFailure records a failed request for the circuit breaker.
func RecordCircuitBreakerFailure(channelID int) {
	cfg := GetCircuitBreakerConfig()
	if !cfg.Enabled {
		return
	}

	cb := getOrCreateBreaker(channelID)

	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	switch state {
	case CircuitBreakerClosed:
		count := atomic.AddInt32(&cb.failureCount, 1)
		atomic.StoreInt64(&cb.lastFailureTime, time.Now().UnixNano())
		if count >= int32(cfg.FailureThreshold) {
			if atomic.CompareAndSwapInt32(&cb.state, int32(CircuitBreakerClosed), int32(CircuitBreakerOpen)) {
				logger.LogWarn(nil, fmt.Sprintf("circuit breaker: channel #%d opened (%d consecutive failures)", channelID, count))
				common.SysLog(fmt.Sprintf("circuit breaker: channel #%d opened (%d consecutive failures)", channelID, count))
			}
		}
	case CircuitBreakerHalfOpen:
		atomic.StoreInt64(&cb.lastFailureTime, time.Now().UnixNano())
		if atomic.CompareAndSwapInt32(&cb.state, int32(CircuitBreakerHalfOpen), int32(CircuitBreakerOpen)) {
			atomic.StoreInt32(&cb.successCount, 0)
			logger.LogWarn(nil, fmt.Sprintf("circuit breaker: channel #%d re-opened (half-open failure)", channelID))
		}
	}
}

// ResetCircuitBreaker manually resets a circuit breaker to closed state.
func ResetCircuitBreaker(channelID int) {
	raw, ok := circuitBreakers.Load(channelID)
	if !ok {
		return
	}
	cb := raw.(*channelCircuitBreaker)
	atomic.StoreInt32(&cb.state, int32(CircuitBreakerClosed))
	atomic.StoreInt32(&cb.failureCount, 0)
	atomic.StoreInt32(&cb.successCount, 0)
	atomic.StoreInt64(&cb.lastFailureTime, 0)
}

// GetCircuitBreakerState returns the current state of a channel's circuit breaker.
func GetCircuitBreakerState(channelID int) (CircuitBreakerState, bool) {
	raw, ok := circuitBreakers.Load(channelID)
	if !ok {
		return CircuitBreakerClosed, false
	}
	cb := raw.(*channelCircuitBreaker)
	return CircuitBreakerState(atomic.LoadInt32(&cb.state)), true
}

// GetAllCircuitBreakerStates returns all circuit breaker states.
func GetAllCircuitBreakerStates() map[int]CircuitBreakerState {
	states := make(map[int]CircuitBreakerState)
	circuitBreakers.Range(func(key, value interface{}) bool {
		cb := value.(*channelCircuitBreaker)
		states[key.(int)] = CircuitBreakerState(atomic.LoadInt32(&cb.state))
		return true
	})
	return states
}

// GetOpenCircuitBreakerCount returns the number of open circuit breakers.
func GetOpenCircuitBreakerCount() int {
	count := 0
	circuitBreakers.Range(func(key, value interface{}) bool {
		cb := value.(*channelCircuitBreaker)
		if CircuitBreakerState(atomic.LoadInt32(&cb.state)) == CircuitBreakerOpen {
			count++
		}
		return true
	})
	return count
}