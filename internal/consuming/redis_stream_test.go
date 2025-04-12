//go:build integration

package consuming

import (
	"bytes"
	"context"
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
