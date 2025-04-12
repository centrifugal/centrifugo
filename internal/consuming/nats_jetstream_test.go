//go:build integration

package consuming

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
)

func TestNatsJetStreamConsumer(t *testing.T) {
	url := "nats://localhost:4222"
	subject := "test.subject" + uuid.NewString()
	durable := "test-durable" + uuid.NewString()

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
		Subjects:            []string{subject},
		DurableConsumerName: durable,
		Ordered:             false,         // Change to true to test ordered mode.
		MethodHeader:        "test-method", // This header key will be used to extract the command method.
		// PublicationDataMode remains disabled for this test.
	}

	consumer, err := NewNatsJetStreamConsumer(cfg, dispatcher, testCommon(prometheus.NewRegistry()))
	if err != nil {
		t.Fatalf("failed to create NATS JetStream consumer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}

	waitCh(t, done, 5*time.Second, "timeout waiting for message processing")
}
