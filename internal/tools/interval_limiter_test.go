package tools

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntervalLimiterThrottles(t *testing.T) {
	l := NewIntervalLimiter(time.Hour)
	require.True(t, l.Allow(), "first call should be allowed")
	require.False(t, l.Allow(), "second call within interval should be throttled")
	require.False(t, l.Allow())
}

func TestIntervalLimiterConcurrentSingleAllow(t *testing.T) {
	l := NewIntervalLimiter(time.Hour)
	const n = 200
	var allowed int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if l.Allow() {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), allowed, "exactly one concurrent caller should be allowed per interval")
}
