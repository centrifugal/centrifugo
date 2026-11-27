//go:build integration

package redisqueue

import (
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/rueidis"
	"github.com/stretchr/testify/require"

	"github.com/centrifugal/centrifugo/v6/internal/redisshard"
)

// answerHoleProxy forwards TCP to Redis and, while hole is on, drops what
// Redis answers without closing the connection: an answer lost on a live
// connection (a black hole without a reset).
func answerHoleProxy(t *testing.T, target string) (addr string, hole *atomic.Bool) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	hole = &atomic.Bool{}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			s, err := net.Dial("tcp", target)
			if err != nil {
				_ = c.Close()
				continue
			}
			t.Cleanup(func() { _ = c.Close(); _ = s.Close() })
			go func() { _, _ = io.Copy(s, c) }()
			go func() {
				buf := make([]byte, 32*1024)
				for {
					n, err := s.Read(buf)
					if n > 0 && !hole.Load() {
						if _, werr := c.Write(buf[:n]); werr != nil {
							return
						}
					}
					if err != nil {
						_ = c.Close()
						return
					}
				}
			}()
		}
	}()
	return l.Addr().String(), hole
}

// A blocking read whose answer is lost (the entry delivered to this consumer
// by Redis, the answer never received) ends past its block plus its grace,
// and without reclaim the entry is enqueued from the consumer's pending
// entries. Before, the read waited for good (the consumer stopped consuming),
// and an entry delivered by a lost answer was never processed.
func TestConsumerLostReadAnswer(t *testing.T) {
	const readGrace = 300 * time.Millisecond

	addr, hole := answerHoleProxy(t, "127.0.0.1:6379")
	e, err := redisshard.NewRedisShard(redisshard.RedisShardConfig{Address: addr})
	require.NoError(t, err)
	t.Cleanup(e.Close)
	direct := testRedisShard(t)
	t.Cleanup(direct.Close)
	stream := t.Name() + strconv.FormatInt(time.Now().UnixNano(), 10)

	var processed atomic.Int64
	const block = 500 * time.Millisecond
	c, err := NewConsumer(e, ConsumerOptions{
		Stream:          stream,
		GroupName:       stream,
		ConsumerFunc:    func(*Message) error { processed.Add(1); return nil },
		BlockingTimeout: block,
		Concurrency:     1,
	})
	require.NoError(t, err)
	go func() {
		for range c.Errors {
		}
	}()
	p, err := NewProducer(direct, ProducerOptions{Stream: stream})
	require.NoError(t, err)

	c.readGrace = readGrace
	runDone := make(chan struct{})
	go func() { c.Run(); close(runDone) }()
	t.Cleanup(func() { c.Shutdown(); <-runDone })
	require.NoError(t, p.Enqueue(&Message{Values: map[string]string{"n": "1"}}))
	require.Eventually(t, func() bool { return processed.Load() == 1 }, 5*time.Second, 10*time.Millisecond, "works before the hole")

	// a read is blocked (the consumer is idle); its answer is lost
	time.Sleep(block / 5)
	hole.Store(true)
	require.NoError(t, p.Enqueue(&Message{Values: map[string]string{"n": "2"}}))
	require.Eventually(t, func() bool {
		res := direct.RunOp(func(client rueidis.Client) rueidis.RedisResult {
			return client.Do(t.Context(), client.B().Xpending().Key(stream).Group(stream).Build())
		})
		arr, err := res.ToArray()
		if err != nil || len(arr) == 0 {
			return false
		}
		n, _ := arr[0].AsInt64()
		return n == 1
	}, 5*time.Second, 10*time.Millisecond, "the entry was delivered to the consumer")
	time.Sleep(block + readGrace + 200*time.Millisecond) // the lost read ends
	hole.Store(false)

	require.Eventually(t, func() bool { return processed.Load() == 2 }, 10*time.Second, 10*time.Millisecond,
		"the entry delivered by the lost answer is processed")
	require.NoError(t, p.Enqueue(&Message{Values: map[string]string{"n": "3"}}))
	require.Eventually(t, func() bool { return processed.Load() == 3 }, 10*time.Second, 10*time.Millisecond,
		"reads work again after the hole")
	require.Equal(t, int64(3), processed.Load(), "nothing processed twice")
}
