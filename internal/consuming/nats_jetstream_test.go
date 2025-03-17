//go:build integration

package consuming

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestNatsJetStreamConsumer(t *testing.T) {
	// Set NATS server URL.
	url := "nats://localhost:4222"
	subject := "test.subject"
	durable := "test-durable"

	// Connect to the NATS server.
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// Create a JetStream context.
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to create JetStream context: %v", err)
	}

	// Ensure a stream exists for our subject.
	streamName := "TEST_STREAM"
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

	// Create a done channel to signal message processing.
	done := make(chan struct{})

	// Create a dummy dispatcher that signals done when a message is processed.
	dispatcher := &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			close(done)
			return nil
		},
	}

	// Create consumer configuration.
	cfg := NatsJetStreamConsumerConfig{
		URL:                 url,
		Subjects:            []string{subject},
		DurableConsumerName: durable,
		Ordered:             false,         // Change to true to test ordered mode.
		MethodHeader:        "test-method", // This header key will be used to extract the command method.
		// PublicationDataMode remains disabled for this test.
	}

	// Create the consumer.
	consumer, err := NewNatsJetStreamConsumer("test", cfg, dispatcher)
	if err != nil {
		t.Fatalf("failed to create NATS JetStream consumer: %v", err)
	}

	// Run the consumer in a separate goroutine.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		if err := consumer.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("consumer error: %v", err)
		}
	}()

	// Publish a message. Using a header (test-method) to trigger command mode.
	msg := nats.NewMsg(subject)
	msg.Data = []byte("Hello, NATS JetStream!")
	msg.Header.Set("test-method", "myMethod")
	_, err = js.PublishMsg(msg)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	waitCh(t, done, 5*time.Second, "timeout waiting for message processing")
}
