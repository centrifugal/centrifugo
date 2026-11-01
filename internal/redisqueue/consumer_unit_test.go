package redisqueue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Shutdown neither blocks nor fails when called again, with and without
// reclaim, and cancels a blocking read.
func TestShutdownIdempotent(t *testing.T) {
	for _, visibilityTimeout := range []time.Duration{0, time.Minute} {
		t.Run(visibilityTimeout.String(), func(t *testing.T) {
			c, err := newConsumer(nil, ConsumerOptions{
				Stream:            "stream",
				GroupName:         "group",
				ConsumerFunc:      func(*Message) error { return nil },
				VisibilityTimeout: visibilityTimeout,
				Concurrency:       1,
			})
			require.NoError(t, err)
			done := make(chan struct{})
			go func() {
				defer close(done)
				c.Shutdown()
				c.Shutdown()
				c.Shutdown()
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				require.Fail(t, "Shutdown blocked")
			}
			require.Error(t, c.stopCtx.Err())
		})
	}
}

func TestNewConsumerOptions(t *testing.T) {
	_, err := newConsumer(nil, ConsumerOptions{GroupName: "group", ConsumerFunc: func(*Message) error { return nil }})
	require.Error(t, err)
	_, err = newConsumer(nil, ConsumerOptions{Stream: "stream", ConsumerFunc: func(*Message) error { return nil }})
	require.Error(t, err)
	_, err = newConsumer(nil, ConsumerOptions{Stream: "stream", GroupName: "group"})
	require.Error(t, err)
	c, err := newConsumer(nil, ConsumerOptions{Stream: "stream", GroupName: "group", ConsumerFunc: func(*Message) error { return nil }})
	require.NoError(t, err)
	require.Equal(t, 15*time.Second, c.options.BlockingTimeout)
	require.Equal(t, 5*time.Second, c.options.ReclaimInterval)
	require.NotEmpty(t, c.options.Name)
}
