package redisqueue

import (
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewSignalHandler(t *testing.T) {
	t.Run("closes the returned channel on SIGINT", func(tt *testing.T) {
		ch := newSignalHandler()

		err := syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		require.NoError(tt, err)

		select {
		case <-time.After(2 * time.Second):
			t.Error("timed out waiting for signal")
		case <-ch:
		}
	})
}
