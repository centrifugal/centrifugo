package timers

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// waitTimer waits for a timer to fire or a timeout, returning true if fired.
func waitTimer(tm *time.Timer, timeout time.Duration) bool {
	select {
	case <-tm.C:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestTimersReliable(t *testing.T) {
	t.Run("acquire timer does not fire immediately", func(t *testing.T) {
		tm := AcquireTimer(50 * time.Millisecond)
		require.NotNil(t, tm)

		fired := waitTimer(tm, 10*time.Millisecond)
		require.False(t, fired, "timer should not have fired immediately")

		ReleaseTimer(tm)
	})

	t.Run("timer fires after duration", func(t *testing.T) {
		tm := AcquireTimer(20 * time.Millisecond)
		require.NotNil(t, tm)

		fired := waitTimer(tm, 100*time.Millisecond)
		require.True(t, fired, "timer did not fire as expected")

		ReleaseTimer(tm)
	})

	t.Run("release stops timer", func(t *testing.T) {
		tm := AcquireTimer(50 * time.Millisecond)
		require.NotNil(t, tm)

		ReleaseTimer(tm)

		fired := waitTimer(tm, 60*time.Millisecond)
		require.False(t, fired, "timer should have been stopped")
	})

	t.Run("reuse timer from pool", func(t *testing.T) {
		// Acquire and release a timer to populate the pool
		tm1 := AcquireTimer(50 * time.Millisecond)
		ReleaseTimer(tm1)

		// Acquire again from pool
		tm2 := AcquireTimer(20 * time.Millisecond)
		require.NotNil(t, tm2)

		fired := waitTimer(tm2, 100*time.Millisecond)
		require.True(t, fired, "reused timer should have fired")

		ReleaseTimer(tm2)
	})

	t.Run("concurrent acquire and release", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 50

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				tm := AcquireTimer(10 * time.Millisecond)
				// deterministically wait for timer to fire or grace period
				waitTimer(tm, 20*time.Millisecond)
				ReleaseTimer(tm)
			}()
		}

		wg.Wait()
	})

	t.Run("release timer that already fired", func(t *testing.T) {
		tm := AcquireTimer(10 * time.Millisecond)
		require.NotNil(t, tm)

		fired := waitTimer(tm, 50*time.Millisecond)
		require.True(t, fired, "timer should have fired")

		ReleaseTimer(tm)

		// Acquire again to ensure pool is working
		tm2 := AcquireTimer(20 * time.Millisecond)
		require.NotNil(t, tm2)
		ReleaseTimer(tm2)
	})
}
