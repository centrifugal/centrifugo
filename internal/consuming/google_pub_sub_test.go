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
	"github.com/stretchr/testify/require"
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

// TestGooglePubSubConsumer_DrainAcksInFlight verifies that a message in flight
// when the consumer is stopped is handled and acknowledged: the subscription
// does not deliver it again.
func TestGooglePubSubConsumer_DrainAcksInFlight(t *testing.T) {
	// Point the Pub/Sub client to the emulator.
	t.Setenv("PUBSUB_EMULATOR_HOST", "localhost:8085")

	ctx := context.Background()

	projectID := "test-project-" + uuid.NewString()
	topicID := "test-topic-" + uuid.NewString()
	subscriptionID := "test-subscription-" + uuid.NewString()

	client, err := pubsub.NewClient(ctx, projectID)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	topic := &pubsubpb.Topic{
		Name: fmt.Sprintf("projects/%s/topics/%s", projectID, topicID),
	}
	_, err = client.TopicAdminClient.CreateTopic(ctx, topic)
	require.NoError(t, err)

	// The minimal ack deadline: a message left unacknowledged is redelivered
	// once its lease, extended by the client for at most this long, expires.
	const ackDeadline = 10 * time.Second
	_, err = client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:               fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID),
		Topic:              topic.Name,
		AckDeadlineSeconds: int32(ackDeadline.Seconds()),
	})
	require.NoError(t, err)

	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()
	publish := func(data string) {
		_, err := publisher.Publish(ctx, &pubsub.Message{Data: []byte(data)}).Get(ctx)
		require.NoError(t, err)
	}

	cfg := GooglePubSubConsumerConfig{
		ProjectID:              projectID,
		Subscriptions:          []string{subscriptionID},
		MaxOutstandingMessages: 10,
	}

	dispatching := make(chan struct{})
	release := make(chan struct{})
	var dispatchOnce sync.Once
	consumer, err := NewGooglePubSubConsumer(cfg, &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			dispatchOnce.Do(func() {
				close(dispatching)
				<-release
			})
			return nil
		},
	}, testCommon(prometheus.NewRegistry()))
	require.NoError(t, err)

	runCtx, runCancel := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() { runDone <- consumer.Run(runCtx) }()

	publish("first")

	waitCh(t, dispatching, 10*time.Second, "timeout waiting for dispatch")
	runCancel()
	closeDone := make(chan error, 1)
	go func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		closeDone <- consumer.Close(closeCtx)
	}()
	close(release)
	require.NoError(t, <-closeDone)
	require.ErrorIs(t, <-runDone, context.Canceled)

	// Pull the subscription directly rather than through a second streaming
	// consumer: the emulator sometimes doesn't deliver to streaming pulls opened
	// right after the previous client closed. An unacknowledged first message
	// becomes available again once its ack deadline expires, so keep pulling
	// past it. The marker shows the pulls do receive messages.
	publish("marker")
	subscriptionName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID)
	var markerPulled bool
	var redelivered int
	started := time.Now()
	require.Eventually(t, func() bool {
		pullCtx, pullCancel := context.WithTimeout(ctx, time.Second)
		defer pullCancel()
		resp, err := client.SubscriptionAdminClient.Pull(pullCtx, &pubsubpb.PullRequest{
			Subscription: subscriptionName,
			MaxMessages:  10,
		})
		if err == nil {
			var ackIDs []string
			for _, m := range resp.ReceivedMessages {
				if string(m.Message.Data) == "marker" {
					markerPulled = true
				} else {
					redelivered++
				}
				ackIDs = append(ackIDs, m.AckId)
			}
			if len(ackIDs) > 0 {
				_ = client.SubscriptionAdminClient.Acknowledge(ctx, &pubsubpb.AcknowledgeRequest{
					Subscription: subscriptionName,
					AckIds:       ackIDs,
				})
			}
		}
		return markerPulled && time.Since(started) > ackDeadline+5*time.Second
	}, ackDeadline+30*time.Second, 100*time.Millisecond, "marker not pulled")
	require.Zero(t, redelivered, "message acknowledged on shutdown was delivered again")
}
