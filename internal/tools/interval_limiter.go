package tools

import (
	"sync/atomic"
	"time"
)

// IntervalLimiter permits an action at most once per configured interval. It is
// safe for concurrent use and does no allocation or locking on the hot path -
// just an atomic load (and a single CAS when the action is actually allowed),
// so guarding a rare log line with it is effectively free when the interval has
// not elapsed.
type IntervalLimiter struct {
	intervalNanos int64
	lastNanos     atomic.Int64
}

// NewIntervalLimiter creates an IntervalLimiter that allows an action at most
// once per interval.
func NewIntervalLimiter(interval time.Duration) *IntervalLimiter {
	return &IntervalLimiter{intervalNanos: int64(interval)}
}

// Allow reports whether the action may run now. Across all concurrent callers it
// returns true at most once per interval. The first call always returns true.
func (l *IntervalLimiter) Allow() bool {
	now := time.Now().UnixNano()
	last := l.lastNanos.Load()
	if now-last < l.intervalNanos {
		return false
	}
	// Only the caller that swaps in the new timestamp is allowed to proceed, so
	// concurrent callers in the same window can't all log.
	return l.lastNanos.CompareAndSwap(last, now)
}
