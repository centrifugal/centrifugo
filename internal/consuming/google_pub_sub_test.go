//go:build integration

package consuming

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/protobuf/types/known/durationpb"

	"cloud.google.com/go/pubsub/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

func TestGooglePubSubConsumer(t *testing.T) {
	// Point the Pub/Sub client to the emulator.
	_ = os.Setenv("PUBSUB_EMULATOR_HOST", "localhost:8085")
	defer func() {
		_ = os.Unsetenv("PUBSUB_EMULATOR_HOST")
	}()

	ctx := context.Background()

	// Setup project, topic, and subscription IDs for testing.
	projectID := "test-project-" + uuid.NewString()
	topicID := "test-topic-" + uuid.NewString()
	subscriptionID := "test-subscription-" + uuid.NewString()

	// Create a Pub/Sub client (will connect to the emulator).
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to create pubsub client: %v", err)
	}

	topic := &pubsubpb.Topic{
		Name: fmt.Sprintf("projects/%s/topics/%s", projectID, topicID),
	}
	_, err = client.TopicAdminClient.CreateTopic(ctx, topic)
	if err != nil {
		t.Fatalf("failed to create topic: %v", err)
	}

	s := &pubsubpb.Subscription{
		Name:                          fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID),
		Topic:                         topic.Name,
		TopicMessageRetentionDuration: durationpb.New(1 * time.Hour),
		EnableMessageOrdering:         true,
		AckDeadlineSeconds:            20,
	}
	_, err = client.SubscriptionAdminClient.CreateSubscription(ctx, s)
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	publisher := client.Publisher(topic.Name)
	publisher.EnableMessageOrdering = true

	// Publish a message with an ordering key.
	orderingKey := "order-1"
	result := publisher.Publish(ctx, &pubsub.Message{
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
		Subscriptions:          []string{subscriptionID},
		MaxOutstandingMessages: 10,
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
	consumer, err := NewGooglePubSubConsumer(config, dispatcher, testCommon(prometheus.NewRegistry()))
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
