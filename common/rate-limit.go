package common

import (
	"sync"
	"time"
)

type rateLimitEntry struct {
	id uint64
	at int64
}

// RateLimitReservation holds a slot reserved by Reserve until it is committed
// or rolled back. A reservation is safe to settle from a different goroutine
// than the one that created it.
type RateLimitReservation struct {
	limiter *InMemoryRateLimiter
	key     string
	id      uint64
	state   rateLimitReservationState
}

type rateLimitReservationState uint8

const (
	rateLimitReservationNoop rateLimitReservationState = iota
	rateLimitReservationPending
	rateLimitReservationCommitted
	rateLimitReservationRolledBack
)

type InMemoryRateLimiter struct {
	store              map[string][]rateLimitEntry
	mutex              sync.Mutex
	expirationDuration time.Duration
	nextEntryID        uint64
	cleanupStarted     bool
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.store != nil {
		return
	}

	l.store = make(map[string][]rateLimitEntry)
	l.expirationDuration = expirationDuration
	if expirationDuration > 0 && !l.cleanupStarted {
		l.cleanupStarted = true
		go l.clearExpiredItems()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key, queue := range l.store {
			queue = removeExpiredEntries(queue, now, int64(l.expirationDuration.Seconds()))
			if len(queue) == 0 {
				delete(l.store, key)
				continue
			}
			l.store[key] = queue
		}
		l.mutex.Unlock()
	}
}

func removeExpiredEntries(queue []rateLimitEntry, now, duration int64) []rateLimitEntry {
	if duration <= 0 {
		return queue
	}
	firstLive := 0
	for firstLive < len(queue) && now-queue[firstLive].at >= duration {
		firstLive++
	}
	if firstLive == 0 {
		return queue
	}
	if firstLive == len(queue) {
		return nil
	}
	return queue[firstLive:]
}

// Reserve atomically reserves one request slot. The slot remains counted until
// Commit or Rollback is called. A maxRequestNum of zero disables the limit.
func (l *InMemoryRateLimiter) Reserve(key string, maxRequestNum int, duration int64) (*RateLimitReservation, bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.store == nil {
		l.store = make(map[string][]rateLimitEntry)
	}
	if maxRequestNum <= 0 {
		return &RateLimitReservation{limiter: l, state: rateLimitReservationNoop}, true
	}

	now := time.Now().Unix()
	queue := removeExpiredEntries(l.store[key], now, duration)
	if len(queue) >= maxRequestNum {
		l.store[key] = queue
		return nil, false
	}

	l.nextEntryID++
	entry := rateLimitEntry{id: l.nextEntryID, at: now}
	l.store[key] = append(queue, entry)
	return &RateLimitReservation{
		limiter: l,
		key:     key,
		id:      entry.id,
		state:   rateLimitReservationPending,
	}, true
}

// Commit keeps the reserved slot counted. It is safe to call more than once.
func (r *RateLimitReservation) Commit() {
	if r == nil || r.limiter == nil {
		return
	}

	r.limiter.mutex.Lock()
	defer r.limiter.mutex.Unlock()
	if r.state == rateLimitReservationPending {
		r.state = rateLimitReservationCommitted
	}
}

// Rollback releases the reserved slot. It is safe to call more than once and
// has no effect after Commit.
func (r *RateLimitReservation) Rollback() {
	if r == nil || r.limiter == nil {
		return
	}

	r.limiter.mutex.Lock()
	defer r.limiter.mutex.Unlock()
	if r.state != rateLimitReservationPending {
		return
	}

	queue := r.limiter.store[r.key]
	for i, entry := range queue {
		if entry.id == r.id {
			queue = append(queue[:i], queue[i+1:]...)
			if len(queue) == 0 {
				delete(r.limiter.store, r.key)
			} else {
				r.limiter.store[r.key] = queue
			}
			break
		}
	}
	r.state = rateLimitReservationRolledBack
}

// Request preserves the original immediate-consumption API.
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	reservation, allowed := l.Reserve(key, maxRequestNum, duration)
	if !allowed {
		return false
	}
	reservation.Commit()
	return true
}

// Cancel removes one previously admitted request from the in-memory window.
// It is retained for callers using the legacy key-based rollback behavior.
func (l *InMemoryRateLimiter) Cancel(key string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	queue, ok := l.store[key]
	if !ok || len(queue) == 0 {
		return
	}
	l.store[key] = queue[:len(queue)-1]
	if len(l.store[key]) == 0 {
		delete(l.store, key)
	}
}
