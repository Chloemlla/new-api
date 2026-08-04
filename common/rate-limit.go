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

// Exported aliases for testing.
const (
	RateLimitReservationNoop      = rateLimitReservationNoop
	RateLimitReservationPending   = rateLimitReservationPending
	RateLimitReservationCommitted = rateLimitReservationCommitted
	RateLimitReservationRolledBack = rateLimitReservationRolledBack
)

// State returns the reservation's current state.
func (r *RateLimitReservation) State() rateLimitReservationState {
	if r == nil {
		return rateLimitReservationNoop
	}
	return r.state
}

// inMemoryRateLimiterShard holds a portion of the key space. Using 64 shards
// with FNV-1a hashing means concurrent rate-limit checks on different keys
// almost never contend on the same mutex.
type inMemoryRateLimiterShard struct {
	mutex sync.Mutex
	store map[string][]rateLimitEntry
}

const numRateLimiterShards = 64

// InMemoryRateLimiter is a sharded in-memory rate limiter. Each key is mapped
// to one of 64 shards via FNV-1a hashing, so concurrent operations on
// different keys proceed without lock contention.
type InMemoryRateLimiter struct {
	shards             [numRateLimiterShards]inMemoryRateLimiterShard
	expirationDuration time.Duration
	nextEntryID        uint64
	cleanupOnce        sync.Once
}

func (l *InMemoryRateLimiter) shardForKey(key string) *inMemoryRateLimiterShard {
	h := fnv1a(key)
	return &l.shards[h&(numRateLimiterShards-1)]
}

// fnv1a returns the FNV-1a hash of s. It is allocation-free and fast.
func fnv1a(s string) uint64 {
	var h uint64 = 14695981039346656037 // offset basis
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211 // prime
	}
	return h
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	l.expirationDuration = expirationDuration
	if expirationDuration > 0 {
		l.cleanupOnce.Do(func() {
			go l.clearExpiredItems()
		})
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		now := time.Now().Unix()
		dur := int64(l.expirationDuration.Seconds())
		for i := range l.shards {
			shard := &l.shards[i]
			shard.mutex.Lock()
			for key, queue := range shard.store {
				queue = removeExpiredEntries(queue, now, dur)
				if len(queue) == 0 {
					delete(shard.store, key)
					continue
				}
				shard.store[key] = queue
			}
			shard.mutex.Unlock()
		}
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
	shard := l.shardForKey(key)
	shard.mutex.Lock()

	if shard.store == nil {
		shard.store = make(map[string][]rateLimitEntry)
	}
	if maxRequestNum <= 0 {
		shard.mutex.Unlock()
		return &RateLimitReservation{limiter: l, state: rateLimitReservationNoop}, true
	}

	now := time.Now().Unix()
	queue := removeExpiredEntries(shard.store[key], now, duration)
	if len(queue) >= maxRequestNum {
		shard.store[key] = queue
		shard.mutex.Unlock()
		return nil, false
	}

	l.nextEntryID++
	entry := rateLimitEntry{id: l.nextEntryID, at: now}
	shard.store[key] = append(queue, entry)
	shard.mutex.Unlock()
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

	shard := r.limiter.shardForKey(r.key)
	shard.mutex.Lock()
	defer shard.mutex.Unlock()
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

	shard := r.limiter.shardForKey(r.key)
	shard.mutex.Lock()
	defer shard.mutex.Unlock()
	if r.state != rateLimitReservationPending {
		return
	}

	queue := shard.store[r.key]
	for i, entry := range queue {
		if entry.id == r.id {
			queue = append(queue[:i], queue[i+1:]...)
			if len(queue) == 0 {
				delete(shard.store, r.key)
			} else {
				shard.store[r.key] = queue
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
	shard := l.shardForKey(key)
	shard.mutex.Lock()
	defer shard.mutex.Unlock()

	queue, ok := shard.store[key]
	if !ok || len(queue) == 0 {
		return
	}
	shard.store[key] = queue[:len(queue)-1]
	if len(shard.store[key]) == 0 {
		delete(shard.store, key)
	}
}