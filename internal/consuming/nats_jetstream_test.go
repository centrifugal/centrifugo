//go:build integration

package consuming

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestNatsJetStreamConsumer(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
	}{
		{name: "basic"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testNatsJetStreamConsumer(t)
		})
	}
}

func testNatsJetStreamConsumer(t *testing.T) {
	url := "nats://localhost:4222"
	subject := "test.subject." + uuid.NewString()
	durableConsumerName := "test-durable-" + uuid.NewString()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := nc.JetStream()
	require.NoError(t, err)

	streamName := "TEST_STREAM_" + uuid.NewString()
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
		Storage:  nats.MemoryStorage,
	})
	require.NoError(t, err)

	var receivedNum atomic.Int64

	createConsumer := func(name string, done chan struct{}) *NatsJetStreamConsumer {
		dispatcher := &MockDispatcher{
			onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
				receivedNum.Add(int64(1))
				close(done)
				return nil
			},
		}

		cfg := NatsJetStreamConsumerConfig{
			URL:                 url,
			StreamName:          streamName,
			Subjects:            []string{subject},
			DurableConsumerName: durableConsumerName, // shared durable name
			DeliverPolicy:       "new",
			MethodHeader:        "test-method",
		}

		consumer, err := NewNatsJetStreamConsumer(cfg, dispatcher, testCommon(prometheus.NewRegistry()))
		require.NoError(t, err)
		return consumer
	}

	// Create done channels
	done1 := make(chan struct{})
	done2 := make(chan struct{})

	// Start both consumers
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	consumer1 := createConsumer("consumer1", done1)
	go func() {
		if err := consumer1.Run(ctx1); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer1 error: %v", err)
		}
	}()

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	consumer2 := createConsumer("consumer2", done2)
	go func() {
		if err := consumer2.Run(ctx2); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer2 error: %v", err)
		}
	}()

	// Publish a message
	msg := nats.NewMsg(subject)
	msg.Data = []byte("Hello from dual consumers!")
	msg.Header.Set("test-method", "publish")
	_, err = js.PublishMsg(msg)
	require.NoError(t, err)

	waitAnyCh(t, []chan struct{}{done1, done2}, 5*time.Second, "timeout waiting for message processing")
	time.Sleep(500 * time.Millisecond) // Give a short delay in case both try to ack
	require.Equal(t, int64(1), receivedNum.Load(), "only one consumer should have received the message")
}
