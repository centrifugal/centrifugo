//go:build integration

package consuming

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/redisqueue"
	"github.com/centrifugal/centrifugo/v6/internal/redisshard"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestRedisStreamConsumer(t *testing.T) {
	t.Parallel()
	done1 := make(chan struct{})
	done2 := make(chan struct{})

	dispatcher := &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			if bytes.Equal(data, []byte("stream1")) {
				close(done1)
			} else if bytes.Equal(data, []byte("stream2")) {
				close(done2)
			}
			return nil
		},
	}

	streamName1 := "TEST_STREAM" + uuid.NewString()
	streamName2 := "TEST_STREAM" + uuid.NewString()

	cfg := RedisStreamConsumerConfig{
		Redis: configtypes.Redis{
			Address: []string{"localhost:6379"},
		},
		Streams:       []string{streamName1, streamName2},
		ConsumerGroup: uuid.NewString(),
		PayloadValue:  "payload",
		NumWorkers:    1,
	}

	consumer, err := NewRedisStreamConsumer(
		cfg, dispatcher, testCommon(prometheus.NewRegistry()))
	if err != nil {
		t.Fatalf("failed to create Redis Stream consumer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		if err := consumer.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("consumer error: %v", err)
		}
	}()

	shards, _, err := redisshard.BuildRedisShards(cfg.Redis)
	require.NoError(t, err)
	require.Len(t, shards, 1)

	producer, err := redisqueue.NewProducer(shards[0], redisqueue.ProducerOptions{
		Stream: streamName1,
	})
	if err != nil {
		t.Fatalf("failed to create producer: %v", err)
	}
	err = producer.Enqueue(&redisqueue.Message{
		Values: map[string]string{
			"payload": "stream1",
		},
	})

	producer, err = redisqueue.NewProducer(shards[0], redisqueue.ProducerOptions{
		Stream: streamName2,
	})
	if err != nil {
		t.Fatalf("failed to create producer: %v", err)
	}
	err = producer.Enqueue(&redisqueue.Message{
		Values: map[string]string{
			"payload": "stream2",
		},
	})

	waitCh(t, done1, 5*time.Second, "timeout waiting for message processing")
	waitCh(t, done2, 5*time.Second, "timeout waiting for message processing")
}

func TestRedisStreamConsumer_OnlyOneGetsMessage(t *testing.T) {
	t.Parallel()
	done := make(chan string, 1)

	var receivedNum atomic.Int64

	dispatcher := func(name string) *MockDispatcher {
		return &MockDispatcher{
			onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
				receivedNum.Add(1)
				close(done)
				return nil
			},
		}
	}

	streamName := "TEST_STREAM_" + uuid.NewString()
	consumerGroup := "group_" + uuid.NewString()

	cfg := RedisStreamConsumerConfig{
		Redis: configtypes.Redis{
			Address: []string{"localhost:6379"},
		},
		Streams:       []string{streamName},
		ConsumerGroup: consumerGroup,
		PayloadValue:  "payload",
		NumWorkers:    1,
	}

	// Build shards once to share producer
	shards, _, err := redisshard.BuildRedisShards(configtypes.Redis{
		Address: []string{"localhost:6379"},
	})
	require.NoError(t, err)

	// Start first consumer
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	consumer1, err := NewRedisStreamConsumer(cfg, dispatcher("consumer1"), testCommon(prometheus.NewRegistry()))
	require.NoError(t, err)
	go func() {
		if err := consumer1.Run(ctx1); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer1 error: %v", err)
		}
	}()

	// Start second consumer
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	consumer2, err := NewRedisStreamConsumer(cfg, dispatcher("consumer2"), testCommon(prometheus.NewRegistry()))
	require.NoError(t, err)
	go func() {
		if err := consumer2.Run(ctx2); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer2 error: %v", err)
		}
	}()

	// Send message.
	producer, err := redisqueue.NewProducer(shards[0], redisqueue.ProducerOptions{
		Stream: streamName,
	})
	require.NoError(t, err)
	err = producer.Enqueue(&redisqueue.Message{
		Values: map[string]string{
			"payload": "test",
		},
	})
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message processing")
	}
	time.Sleep(500 * time.Millisecond) // prevent any second dispatch.
	require.Equal(t, int64(1), receivedNum.Load(), "only one consumer should process the message")
}
