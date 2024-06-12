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

func produceTestMessageToPartition(topic string, message []byte, partition int32) error {
	// Create a new client.
	client, err := kgo.NewClient(
		kgo.SeedBrokers(testKafkaBrokerURL),
		kgo.RecordPartitioner(kgo.ManualPartitioner()),
	)
	if err != nil {
		return fmt.Errorf("failed to create Kafka client: %w", err)
	}
	defer client.Close()

	// Produce a message until we hit desired partition.
	res := client.ProduceSync(context.Background(), &kgo.Record{
		Topic: topic, Partition: partition, Value: message})
	if res.FirstErr() != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}
	producedPartition := res[0].Record.Partition
	if producedPartition != partition {
		return fmt.Errorf("failed to produce message to partition %d, produced to %d", partition, producedPartition)
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

	consumer, err := NewKafkaConsumer("test", uuid.NewString(), &MockLogger{}, &MockDispatcher{
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
		consumer, err := NewKafkaConsumer("test", uuid.NewString(), &MockLogger{}, &MockDispatcher{
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
	consumer, err := NewKafkaConsumer("test", uuid.NewString(), &MockLogger{}, mockDispatcher, config)
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

// In this test we simulate a scenario where a partition is blocked and the partition consumer
// is stuck on it. We want to make sure that the consumer is not blocked and can still process
// messages from other topics.
func TestKafkaConsumer_BlockedPartitionDoesNotBlockAnotherTopic(t *testing.T) {
	t.Parallel()
	testKafkaTopic1 := "consumer_test_1_" + uuid.New().String()
	testKafkaTopic2 := "consumer_test_1_" + uuid.New().String()

	testPayload1 := []byte(`{"key":"value1"}`)
	testPayload2 := []byte(`{"key":"value2"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := createTestTopic(ctx, testKafkaTopic1, 1, 1)
	require.NoError(t, err)

	err = createTestTopic(ctx, testKafkaTopic2, 1, 1)
	require.NoError(t, err)

	event1Received := make(chan struct{})
	event2Received := make(chan struct{})
	consumerClosed := make(chan struct{})
	doneCh := make(chan struct{})

	config := KafkaConfig{
		Brokers:       []string{testKafkaBrokerURL},
		Topics:        []string{testKafkaTopic1, testKafkaTopic2},
		ConsumerGroup: uuid.New().String(),
	}

	numCalls := 0

	mockDispatcher := &MockDispatcher{
		onDispatch: func(ctx context.Context, method string, data []byte) error {
			if numCalls == 0 {
				numCalls++
				close(event1Received)
				// Block till the event2 received. This must not block the consumer and event2
				// must still be processed successfully.
				<-event2Received
				return nil
			}
			close(event2Received)
			return nil
		},
	}
	consumer, err := NewKafkaConsumer("test", uuid.NewString(), &MockLogger{}, mockDispatcher, config)
	require.NoError(t, err)

	go func() {
		err = produceTestMessage(testKafkaTopic1, testPayload1)
		require.NoError(t, err)

		// Wait until the first message is received to make sure messages read by separate PollRecords calls.
		<-event1Received
		err = produceTestMessage(testKafkaTopic2, testPayload2)
		require.NoError(t, err)
	}()

	go func() {
		err := consumer.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		close(consumerClosed)
	}()

	waitCh(t, event2Received, 30*time.Second, "timeout waiting for event 2")
	cancel()
	waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
	close(doneCh)
}

// In this test we simulate a scenario where a partition is blocked and the partition consumer
// is stuck on it. We want to make sure that the consumer is not blocked and can still process
// messages from other topic partitions.
func TestKafkaConsumer_BlockedPartitionDoesNotBlockAnotherPartition(t *testing.T) {
	partitionBufferSizes := []int{-1, 0}

	for _, partitionBufferSize := range partitionBufferSizes {
		t.Run(fmt.Sprintf("partition_buffer_size_%d", partitionBufferSize), func(t *testing.T) {
			t.Parallel()
			testKafkaTopic1 := "consumer_test_1_" + uuid.New().String()

			testPayload1 := []byte(`{"key":"value1"}`)
			testPayload2 := []byte(`{"key":"value2"}`)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := createTestTopic(ctx, testKafkaTopic1, 2, 1)
			require.NoError(t, err)

			event1Received := make(chan struct{})
			event2Received := make(chan struct{})
			consumerClosed := make(chan struct{})
			doneCh := make(chan struct{})

			config := KafkaConfig{
				Brokers:             []string{testKafkaBrokerURL},
				Topics:              []string{testKafkaTopic1},
				ConsumerGroup:       uuid.New().String(),
				PartitionBufferSize: partitionBufferSize,
			}

			numCalls := 0

			mockDispatcher := &MockDispatcher{
				onDispatch: func(ctx context.Context, method string, data []byte) error {
					if numCalls == 0 {
						numCalls++
						close(event1Received)
						// Block till the event2 received. This must not block the consumer and event2
						// must still be processed successfully.
						<-event2Received
						return nil
					}
					close(event2Received)
					return nil
				},
			}
			consumer, err := NewKafkaConsumer("test", uuid.NewString(), &MockLogger{}, mockDispatcher, config)
			require.NoError(t, err)

			go func() {
				err = produceTestMessageToPartition(testKafkaTopic1, testPayload1, 0)
				require.NoError(t, err)

				// Wait until the first message is received to make sure messages read by separate PollRecords calls.
				<-event1Received
				err = produceTestMessageToPartition(testKafkaTopic1, testPayload2, 1)
				require.NoError(t, err)
			}()

			go func() {
				err := consumer.Run(ctx)
				require.ErrorIs(t, err, context.Canceled)
				close(consumerClosed)
			}()

			waitCh(t, event2Received, 30*time.Second, "timeout waiting for event 2")
			cancel()
			waitCh(t, consumerClosed, 30*time.Second, "timeout waiting for consumer closed")
			close(doneCh)
		})
	}
}
