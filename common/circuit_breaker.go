package common

import (
	"fmt"
	"sync"
	"time"
)

// CircuitBreakerEnabled is the global toggle for the channel circuit breaker.
var CircuitBreakerEnabled bool

// CircuitBreakerFailureThreshold is the number of consecutive failures required
// to open the circuit breaker for a channel.
var CircuitBreakerFailureThreshold = 5

// CircuitBreakerCooldownSeconds is the duration (in seconds) the circuit breaker
// stays open before transitioning to half-open.
var CircuitBreakerCooldownSeconds int64 = 60

// ChannelBreaker is the global circuit breaker instance for channel-level
// fast-fail protection.
var ChannelBreaker = &channelBreaker{}

// channelBreaker provides per-channel circuit breaking with three states:
// Closed (normal), Open (rejecting), and Half-Open (probing recovery).
type channelBreaker struct {
	mu       sync.Mutex
	breakers map[int]*channelBreakerEntry
}

type channelBreakerState int32

const (
	breakerClosed   channelBreakerState = 0
	breakerOpen     channelBreakerState = 1
	breakerHalfOpen channelBreakerState = 2
)

type channelBreakerEntry struct {
	state           channelBreakerState
	failureCount    int
	successCount    int
	lastFailureTime int64
}

// RecordSuccess records a successful request for the given channel.
func (cb *channelBreaker) RecordSuccess(channelID int) {
	if !CircuitBreakerEnabled {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	entry := cb.getOrCreate(channelID)
	switch entry.state {
	case breakerClosed:
		entry.failureCount = 0
	case breakerHalfOpen:
		entry.successCount++
		if entry.successCount >= CircuitBreakerFailureThreshold {
			entry.state = breakerClosed
			entry.failureCount = 0
			entry.successCount = 0
			SysLog(fmt.Sprintf("circuit breaker: channel #%d closed (recovered)", channelID))
		}
	}
}

// RecordFailure records a failed request for the given channel.
func (cb *channelBreaker) RecordFailure(channelID int) {
	if !CircuitBreakerEnabled {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	entry := cb.getOrCreate(channelID)
	switch entry.state {
	case breakerClosed:
		entry.failureCount++
		if entry.failureCount >= CircuitBreakerFailureThreshold {
			entry.state = breakerOpen
			entry.lastFailureTime = nowUnix()
			SysLog(fmt.Sprintf("circuit breaker: channel #%d opened (%d consecutive failures)", channelID, entry.failureCount))
		}
	case breakerHalfOpen:
		entry.state = breakerOpen
		entry.lastFailureTime = nowUnix()
		entry.successCount = 0
		SysLog(fmt.Sprintf("circuit breaker: channel #%d re-opened (half-open failure)", channelID))
	}
}

// IsBlocked returns true when the circuit breaker for the given channel is open
// and the cooldown period has not elapsed. A blocked channel should be excluded
// from routing.
func (cb *channelBreaker) IsBlocked(channelID int) bool {
	if !CircuitBreakerEnabled {
		return false
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	entry, ok := cb.breakers[channelID]
	if !ok {
		return false
	}
	if entry.state != breakerOpen {
		return false
	}
	if nowUnix()-entry.lastFailureTime >= CircuitBreakerCooldownSeconds {
		// Transition to half-open.
		entry.state = breakerHalfOpen
		entry.successCount = 0
		return false
	}
	return true
}

func (cb *channelBreaker) getOrCreate(channelID int) *channelBreakerEntry {
	if cb.breakers == nil {
		cb.breakers = make(map[int]*channelBreakerEntry)
	}
	entry, ok := cb.breakers[channelID]
	if !ok {
		entry = &channelBreakerEntry{}
		cb.breakers[channelID] = entry
	}
	return entry
}

// ConfigureCircuitBreaker updates the circuit breaker settings and recreates the
// breaker map when the enabled flag changes.
func ConfigureCircuitBreaker(enabled bool, failureThreshold int, cooldownSeconds int64) {
	CircuitBreakerEnabled = enabled
	CircuitBreakerFailureThreshold = failureThreshold
	CircuitBreakerCooldownSeconds = cooldownSeconds
}

func nowUnix() int64 {
	return time.Now().Unix()
}