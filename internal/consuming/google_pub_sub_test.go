//go:build integration

package consuming

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
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
// when the consumer is stopped is handled and acknowledged, and the next
// consumer of the subscription does not receive it again.
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

	markerReceived := make(chan struct{})
	var markerOnce sync.Once
	var redelivered atomic.Int32
	consumer2, err := NewGooglePubSubConsumer(cfg, &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			if string(data) == "marker" {
				markerOnce.Do(func() { close(markerReceived) })
			} else {
				redelivered.Add(1)
			}
			return nil
		},
	}, testCommon(prometheus.NewRegistry()))
	require.NoError(t, err)
	run2Ctx, run2Cancel := context.WithCancel(ctx)
	run2Done := make(chan error, 1)
	go func() { run2Done <- consumer2.Run(run2Ctx) }()

	publish("marker")
	waitCh(t, markerReceived, 10*time.Second, "timeout waiting for marker")
	// Pub/Sub has no order between the marker and a redelivery, and the ack
	// deadline can not be shorter than 10s: wait past the deadline for an
	// unacknowledged first message to be received again.
	require.Never(t, func() bool {
		return redelivered.Load() > 0
	}, ackDeadline+5*time.Second, 100*time.Millisecond, "message acknowledged on shutdown was delivered again")

	run2Cancel()
	require.NoError(t, consumer2.Close(context.Background()))
	require.ErrorIs(t, <-run2Done, context.Canceled)
}
