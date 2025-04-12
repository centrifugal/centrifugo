//go:build integration

package consuming

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func testNatsJetStreamConsumer(t *testing.T, ordered bool) {
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
		URL:          url,
		StreamName:   streamName,
		Subjects:     []string{subject},
		Ordered:      ordered,
		MethodHeader: "test-method", // This header key will be used to extract the command method.
		// PublicationDataMode remains disabled for this test.
	}
	if !ordered {
		cfg.DurableConsumerName = durableConsumerName
	}
	fmt.Println(ordered, cfg.DurableConsumerName)

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
	testCases := []struct {
		name    string
		ordered bool
	}{
		{name: "unordered", ordered: false},
		{name: "ordered", ordered: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testNatsJetStreamConsumer(t, tc.ordered)
		})
	}
}
