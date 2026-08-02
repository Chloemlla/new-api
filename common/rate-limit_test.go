package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterReservationCommitAndRollback(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	reservation, allowed := limiter.Reserve("user", 1, 60)
	require.True(t, allowed)
	require.NotNil(t, reservation)

	_, allowed = limiter.Reserve("user", 1, 60)
	require.False(t, allowed, "a pending reservation must consume its slot")

	reservation.Rollback()
	reservation.Rollback()
	reservation.Commit()

	committed, allowed := limiter.Reserve("user", 1, 60)
	require.True(t, allowed, "rollback must release the reserved slot")
	committed.Commit()

	committed.Rollback()
	_, allowed = limiter.Reserve("user", 1, 60)
	require.False(t, allowed, "rollback after commit must not release a committed slot")
}

func TestInMemoryRateLimiterRollbackRemovesItsOwnReservation(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	first, allowed := limiter.Reserve("user", 2, 60)
	require.True(t, allowed)
	second, allowed := limiter.Reserve("user", 2, 60)
	require.True(t, allowed)

	first.Commit()
	second.Rollback()

	replacement, allowed := limiter.Reserve("user", 2, 60)
	require.True(t, allowed)
	replacement.Commit()

	_, allowed = limiter.Reserve("user", 2, 60)
	require.False(t, allowed, "the committed first reservation and replacement fill the limit")
}

func TestInMemoryRateLimiterRequestRemainsImmediate(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	require.True(t, limiter.Request("user", 1, 60))
	require.False(t, limiter.Request("user", 1, 60))
}
