//go:build integration

package consuming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	testKafkaBrokerURL = "localhost:29092"
)

// MockDispatcher implements the Dispatcher interface for testing.
type MockDispatcher struct {
	onDispatch func(ctx context.Context, method string, data []byte) error
}

func (m *MockDispatcher) Dispatch(ctx context.Context, method string, data []byte) error {
	return m.onDispatch(ctx, method, data)
}

// MockLogger implements the Logger interface for testing.
type MockLogger struct {
	// Add necessary fields to simulate behavior or record calls
}

func (m *MockLogger) LogEnabled(_ centrifuge.LogLevel) bool {
	return true // or false based on your test needs
}

func (m *MockLogger) Log(_ centrifuge.LogEntry) {
	// Implement mock logic, e.g., storing log entries for assertions
}

func produceTestMessage(topic string, message []byte) error {
	// Create a new client
	client, err := kgo.NewClient(kgo.SeedBrokers(testKafkaBrokerURL))
	if err != nil {
		return fmt.Errorf("failed to create Kafka client: %w", err)
	}
	defer client.Close()

	// Produce a message
	err = client.ProduceSync(context.Background(), &kgo.Record{Topic: topic, Partition: 0, Value: message}).FirstErr()
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}
	return nil
}

func createTestTopic(ctx context.Context, topicName string, numPartitions int32, replicationFactor int16) error {
	cl, err := kgo.NewClient(kgo.SeedBrokers(testKafkaBrokerURL))
	if err != nil {
		return fmt.Errorf("failed to create Kafka client: %w", err)
	}
	defer cl.Close()

	client := kadm.NewClient(cl)
	if err != nil {
		return fmt.Errorf("failed to create Kafka admin client: %w", err)
	}
	defer client.Close()

	// Create the topic
	resp, err := client.CreateTopics(ctx, numPartitions, replicationFactor, nil, topicName)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	for _, topic := range resp.Sorted() {
		if topic.Err != nil {
			if strings.Contains(topic.Err.Error(), "TOPIC_ALREADY_EXISTS") {
				continue
			}
			return fmt.Errorf("failed to create topic '%s': %v", topic.Topic, topic.Err)
		}
	}
	return nil
}

func waitCh(t *testing.T, ch chan struct{}, timeout time.Duration, failureMessage string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		require.Fail(t, failureMessage)
	}
}

func TestKafkaConsumer_GreenScenario(t *testing.T) {
	t.Parallel()
	testKafkaTopic := "centrifugo_consumer_test_" + uuid.New().String()
	testMethod := "method"
	testPayload := []byte(`{"key":"value"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := createTestTopic(ctx, testKafkaTopic, 1, 1)
	require.NoError(t, err)

	config := KafkaConfig{
		Brokers:       []string{testKafkaBrokerURL}, // Adjust as needed
		Topics:        []string{testKafkaTopic},
		ConsumerGroup: uuid.New().String(),
	}

	eventReceived := make(chan struct{})
	consumerClosed := make(chan struct{})

	consumer, err := NewKafkaConsumer(&MockLogger{}, &MockDispatcher{
		onDispatch: func(ctx context.Context, method string, data []byte) error {
			require.Equal(t, testMethod, method)
			require.Equal(t, testPayload, data)
			close(eventReceived)
			return nil
		},
	}, config)
	require.NoError(t, err)

	go func() {
		err := consumer.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		close(consumerClosed)
	}()

	testEvent := KafkaJSONEvent{
		Method:  testMethod,
		Payload: JSONRawOrString(testPayload),
	}
	testMessage, _ := json.Marshal(testEvent)
	err = produceTestMessage(testKafkaTopic, testMessage)
	require.NoError(t, err)

	waitCh(t, eventReceived, 30*time.Second, "timeout waiting for event")
	cancel()
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
}

func TestKafkaConsumer_SeveralConsumers(t *testing.T) {
	t.Parallel()
	testKafkaTopic := "centrifugo_consumer_test_" + uuid.New().String()
	testMethod := "method"
	testPayload := []byte(`{"key":"value"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := createTestTopic(ctx, testKafkaTopic, 1, 1)
	require.NoError(t, err)

	config := KafkaConfig{
		Brokers:       []string{testKafkaBrokerURL}, // Adjust as needed
		Topics:        []string{testKafkaTopic},
		ConsumerGroup: uuid.New().String(),
	}

	eventReceived := make(chan struct{})
	consumerClosed := make(chan struct{})

	for i := 0; i < 3; i++ {
		consumer, err := NewKafkaConsumer(&MockLogger{}, &MockDispatcher{
			onDispatch: func(ctx context.Context, method string, data []byte) error {
				require.Equal(t, testMethod, method)
				require.Equal(t, testPayload, data)
				close(eventReceived)
				return nil
			},
		}, config)
		require.NoError(t, err)

		go func() {
			err := consumer.Run(ctx)
			require.ErrorIs(t, err, context.Canceled)
			consumerClosed <- struct{}{}
		}()
	}

	testEvent := KafkaJSONEvent{
		Method:  testMethod,
		Payload: JSONRawOrString(testPayload),
	}
	testMessage, _ := json.Marshal(testEvent)
	err = produceTestMessage(testKafkaTopic, testMessage)
	require.NoError(t, err)

	waitCh(t, eventReceived, 30*time.Second, "timeout waiting for event")
	cancel()
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
}

func TestKafkaConsumer_RetryAfterDispatchError(t *testing.T) {
	t.Parallel()
	testKafkaTopic := "centrifugo_consumer_test_" + uuid.New().String()
	testMethod := "method"
	testPayload := []byte(`{"key":"value"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := createTestTopic(ctx, testKafkaTopic, 1, 1)
	require.NoError(t, err)

	config := KafkaConfig{
		Brokers:       []string{testKafkaBrokerURL},
		Topics:        []string{testKafkaTopic},
		ConsumerGroup: uuid.New().String(),
	}

	successCh := make(chan struct{})
	consumerClosed := make(chan struct{})

	retryCount := 0
	numFailures := 3

	mockDispatcher := &MockDispatcher{
		onDispatch: func(ctx context.Context, method string, data []byte) error {
			if retryCount < numFailures {
				retryCount++
				return errors.New("dispatch error")
			}
			close(successCh)
			return nil
		},
	}
	consumer, err := NewKafkaConsumer(&MockLogger{}, mockDispatcher, config)
	require.NoError(t, err)

	go func() {
		err := consumer.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		close(consumerClosed)
	}()

	testEvent := KafkaJSONEvent{
		Method:  testMethod,
		Payload: JSONRawOrString(testPayload),
	}
	testMessage, _ := json.Marshal(testEvent)
	err = produceTestMessage(testKafkaTopic, testMessage)
	require.NoError(t, err)

	waitCh(t, successCh, 30*time.Second, "timeout waiting for successful event process")
	require.Equal(t, numFailures, retryCount)
	cancel()
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
}
