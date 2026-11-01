//go:build integration

package redisqueue

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/rueidis"
	"github.com/stretchr/testify/require"
)

// Without reclaim (VisibilityTimeout 0), entries Redis delivered to the
// consumer by a read which Shutdown interrupted, never received, are handled
// and acknowledged before Run returns. Shutdown doesn't wait for the blocking
// read to time out.
func TestShutdownHandlesEntriesOfInterruptedRead(t *testing.T) {
	shard := testRedisShard(t)
	defer shard.Close()
	stream := "redisqueue-shutdown-" + uuid.NewString()
	group := uuid.NewString()
	const name = "consumer"

	var mu sync.Mutex
	var handled []string
	c, err := NewConsumer(shard, ConsumerOptions{
		Stream:    stream,
		GroupName: group,
		Name:      name,
		ConsumerFunc: func(m *Message) error {
			mu.Lock()
			handled = append(handled, m.Values["n"])
			mu.Unlock()
			return nil
		},
		BlockingTimeout: time.Minute,
		Concurrency:     1,
	})
	require.NoError(t, err)
	defer shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
		return client.Do(context.Background(), client.B().Del().Key(stream).Build())
	})

	p, err := NewProducer(shard, ProducerOptions{Stream: stream})
	require.NoError(t, err)
	require.NoError(t, p.Enqueue(&Message{Values: map[string]string{"n": "1"}}))
	require.NoError(t, p.Enqueue(&Message{Values: map[string]string{"n": "2"}}))
	// Delivered to the consumer as by a read whose reply was never received.
	entries, err := shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
		return client.Do(context.Background(), client.B().Xreadgroup().Group(group, name).
			Streams().Key(stream).Id(">").Build())
	}).AsXRead()
	require.NoError(t, err)
	require.Len(t, entries[stream], 2)

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		c.Run()
	}()
	// The consumer is in its blocking read of new entries.
	require.Eventually(t, func() bool {
		list, err := shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			return client.Do(context.Background(), client.B().ClientList().Build())
		}).ToString()
		return err == nil && containsBlockedRead(list)
	}, 5*time.Second, 10*time.Millisecond)

	started := time.Now()
	c.Shutdown()
	c.Shutdown()
	select {
	case <-runDone:
	case <-time.After(10 * time.Second):
		require.Fail(t, "Run did not return")
	}
	require.Less(t, time.Since(started), 5*time.Second)

	mu.Lock()
	require.Equal(t, []string{"1", "2"}, handled)
	mu.Unlock()
	summary, err := shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
		return client.Do(context.Background(), client.B().Xpending().Key(stream).Group(group).Build())
	}).ToArray()
	require.NoError(t, err)
	pending, err := summary[0].AsInt64()
	require.NoError(t, err)
	require.Zero(t, pending)
}

// containsBlockedRead reports whether a CLIENT LIST reply has a client
// blocked in XREADGROUP.
func containsBlockedRead(list string) bool {
	for _, line := range strings.Split(list, "\n") {
		if strings.Contains(line, "cmd=xreadgroup") && strings.Contains(line, "flags=b") {
			return true
		}
	}
	return false
}
