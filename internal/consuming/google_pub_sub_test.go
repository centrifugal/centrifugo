//go:build integration

package consuming

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud.google.com/go/pubsub"
)

func TestGooglePubSubConsumer(t *testing.T) {
	// Point the Pub/Sub client to the emulator.
	_ = os.Setenv("PUBSUB_EMULATOR_HOST", "localhost:8085")
	defer func() {
		_ = os.Unsetenv("PUBSUB_EMULATOR_HOST")
	}()

	ctx := context.Background()

	// Setup project, topic, and subscription IDs for testing.
	projectID := "test-project"
	topicID := "test-topic-" + uuid.NewString()
	subscriptionID := "test-subscription"

	// Create a Pub/Sub client (will connect to the emulator).
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to create pubsub client: %v", err)
	}

	// Create topic. If it already exists, retrieve it.
	var topic *pubsub.Topic
	topic = client.Topic(topicID)
	ok, err := topic.Exists(ctx)
	if err != nil {
		t.Fatalf("failed to check topic existence: %v", err)
	}
	if !ok {
		topic, err = client.CreateTopic(ctx, topicID)
		if err != nil {
			t.Fatalf("failed to create topic: %v", err)
		}
	}
	topic.EnableMessageOrdering = true

	// Create subscription with ordering enabled.
	sub := client.Subscription(subscriptionID)
	ok, err = sub.Exists(ctx)
	if err != nil {
		t.Fatalf("failed to check subscription existence: %v", err)
	}
	if !ok {
		sub, err = client.CreateSubscription(ctx, subscriptionID, pubsub.SubscriptionConfig{
			Topic:                 topic,
			EnableMessageOrdering: true,
			AckDeadline:           20 * time.Second,
		})
		if err != nil {
			t.Fatalf("failed to create subscription: %v", err)
		}
	}

	// Publish a message with an ordering key.
	orderingKey := "order-1"
	result := topic.Publish(ctx, &pubsub.Message{
		Data:        []byte("Hello, Pub/Sub with ordering!"),
		OrderingKey: orderingKey,
		Attributes: map[string]string{
			"example": "value",
		},
	})
	id, err := result.Get(ctx)
	if err != nil {
		t.Fatalf("failed to publish message: %v", err)
	}
	t.Logf("Published a message with ID: %s", id)

	config := GooglePubSubConsumerConfig{
		ProjectID:              projectID,
		SubscriptionID:         subscriptionID,
		MaxOutstandingMessages: 10,
		EnableMessageOrdering:  true,
	}

	done := make(chan struct{})
	// Ensure done is closed only once. PUB/SUB emulator we use in tests is a bit aggressive
	// in sending duplicate messages.
	var closeDoneOnce sync.Once

	dispatcher := &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			closeDoneOnce.Do(func() {
				close(done)
			})
			return nil
		},
	}
	consumer, err := NewGooglePubSubConsumer(ctx, "test", config, dispatcher)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	go func() {
		if err := consumer.Run(ctx); err != nil {
			t.Errorf("consumer error: %v", err)
		}
	}()

	waitCh(t, done, 5*time.Second, "timeout waiting for message processing")
}
