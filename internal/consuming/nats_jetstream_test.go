//go:build integration

package consuming

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func testNatsJetStreamConsumer(t *testing.T) {
	url := "nats://localhost:4222"
	subject := "test.subject" + uuid.NewString()
	durableConsumerName := "test-durable-" + uuid.NewString()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to create JetStream context: %v", err)
	}

	// Ensure a stream exists for our subject.
	streamName := "TEST_STREAM" + uuid.NewString()
	_, err = js.StreamInfo(streamName)
	if err != nil {
		// Assume the stream doesn't exist; create it using in-memory storage.
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: []string{subject},
			Storage:  nats.MemoryStorage,
		})
		if err != nil {
			t.Fatalf("failed to add stream: %v", err)
		}
	}

	done := make(chan struct{})

	dispatcher := &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			close(done)
			return nil
		},
	}

	cfg := NatsJetStreamConsumerConfig{
		URL:                 url,
		StreamName:          streamName,
		Subjects:            []string{subject},
		DurableConsumerName: durableConsumerName,
		DeliverPolicy:       "new",
		MethodHeader:        "test-method", // This header key will be used to extract the command method.
		// PublicationDataMode remains disabled for this test.
	}

	consumer, err := NewNatsJetStreamConsumer(cfg, dispatcher, testCommon(prometheus.NewRegistry()))
	if err != nil {
		t.Fatalf("failed to create NATS JetStream consumer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer error: %v", err)
		}
	}()
	// Publish a message. Using a header (test-method) to specify that payload is PublishRequest.
	msg := nats.NewMsg(subject)
	msg.Data = []byte("Hello, NATS JetStream!")
	msg.Header.Set("test-method", "publish")
	_, err = js.PublishMsg(msg)
	require.NoError(t, err, "failed to publish message")
	waitCh(t, done, 5*time.Second, "timeout waiting for message processing")
}

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

func testNatsJetStreamConsumerConcurrentConsumers(t *testing.T) {
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

	var mu sync.Mutex
	var receivedBy []string

	createConsumer := func(name string, done chan struct{}) *NatsJetStreamConsumer {
		dispatcher := &MockDispatcher{
			onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
				mu.Lock()
				receivedBy = append(receivedBy, name)
				mu.Unlock()
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

	select {
	case <-done1:
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message to be consumed")
	}

	time.Sleep(500 * time.Millisecond) // Give a short delay in case both try to ack

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, receivedBy, 1, "only one consumer should have received the message")
	t.Logf("Message was processed by: %s", receivedBy[0])
}

func TestNatsJetStreamConsumer_ConcurrentConsumers(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
	}{
		{name: "basic"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testNatsJetStreamConsumerConcurrentConsumers(t)
		})
	}
}
