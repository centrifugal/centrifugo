//go:build integration

package consuming

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestNatsJetStreamConsumer(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                string
		useExistingConsumer bool
	}{
		{name: "basic", useExistingConsumer: false},
		{name: "use_existing_consumer", useExistingConsumer: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testNatsJetStreamConsumer(t, tc.useExistingConsumer)
		})
	}
}

func testNatsJetStreamConsumer(t *testing.T, useExistingConsumer bool) {
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

	// If useExistingConsumer is true, create the consumer manually first
	if useExistingConsumer {
		_, err = js.AddConsumer(streamName, &nats.ConsumerConfig{
			Durable: durableConsumerName,
			Name:    durableConsumerName,
		})
		require.NoError(t, err)
	}

	var receivedNum atomic.Int64

	createConsumer := func(name string, done chan struct{}) *NatsJetStreamConsumer {
		dispatcher := &MockDispatcher{
			onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
				receivedNum.Add(int64(1))
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
			UseExistingConsumer: useExistingConsumer,
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

	waitAnyCh(t, []chan struct{}{done1, done2}, 5*time.Second, "timeout waiting for message processing")
	time.Sleep(500 * time.Millisecond) // Give a short delay in case both try to ack
	require.Equal(t, int64(1), receivedNum.Load(), "only one consumer should have received the message")
}

// TestNatsJetStreamConsumer_RecreateOnConsumerDeleted verifies that the consumer
// recovers without a process restart when its JetStream consumer is lost on the
// server side – ex. the stream is deleted and re-created by an operator, or an
// in-memory stream is lost after NATS restart.
func TestNatsJetStreamConsumer_RecreateOnConsumerDeleted(t *testing.T) {
	t.Parallel()

	url := "nats://localhost:4222"
	subject := "test.recreate." + uuid.NewString()
	durableConsumerName := "test-durable-" + uuid.NewString()
	streamName := "TEST_STREAM_" + uuid.NewString()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := nc.JetStream()
	require.NoError(t, err)

	addStream := func() {
		_, err := js.AddStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: []string{subject},
			Storage:  nats.MemoryStorage,
		})
		require.NoError(t, err)
	}
	addStream()

	var receivedNum atomic.Int64
	dispatcher := &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			receivedNum.Add(1)
			return nil
		},
	}

	cfg := NatsJetStreamConsumerConfig{
		URL:                 url,
		StreamName:          streamName,
		Subjects:            []string{subject},
		DurableConsumerName: durableConsumerName,
		DeliverPolicy:       "new",
		MethodHeader:        "test-method",
	}

	var logBuf syncBuffer
	consumer, err := NewNatsJetStreamConsumer(cfg, dispatcher, testCommonWithLogBuf(&logBuf))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer error: %v", err)
		}
	}()

	publishAndWaitReceived := func(stage string) {
		deadline := time.Now().Add(30 * time.Second)
		before := receivedNum.Load()
		for time.Now().Before(deadline) {
			msg := nats.NewMsg(subject)
			msg.Data = []byte("hello")
			msg.Header.Set("test-method", "publish")
			_, _ = js.PublishMsg(msg)
			// Re-publish periodically: a message published before the
			// consumer is (re-)created is not delivered with DeliverPolicy new.
			publishedAt := time.Now()
			for time.Since(publishedAt) < 300*time.Millisecond {
				if receivedNum.Load() > before {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		t.Fatalf("timeout waiting for message processing: %s", stage)
	}

	// Sanity check: consumer works initially.
	publishAndWaitReceived("initial consume")

	// Delete the stream: the durable consumer is destroyed together with it.
	require.NoError(t, js.DeleteStream(streamName))
	// Wait for the consumer to observe the loss.
	require.Eventually(t, func() bool {
		return strings.Contains(logBuf.String(), "consumer unavailable, triggering consumer recreation")
	}, 30*time.Second, 10*time.Millisecond, "consumer did not observe the stream deletion")

	// Re-create the stream (as an operator like NACK would do). The consumer
	// must re-create its durable and resume consuming without a restart.
	addStream()
	publishAndWaitReceived("consume after stream re-creation")
}

// TestNatsJetStreamConsumer_DrainAcksInFlight verifies that a message in flight
// when the consumer is stopped is handled and acknowledged, and the next
// consumer with the same durable does not receive it again.
func TestNatsJetStreamConsumer_DrainAcksInFlight(t *testing.T) {
	t.Parallel()

	url := "nats://localhost:4222"
	subject := "test.drain." + uuid.NewString()
	durableConsumerName := "test-durable-" + uuid.NewString()
	streamName := "TEST_STREAM_" + uuid.NewString()

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := nc.JetStream()
	require.NoError(t, err)
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
		Storage:  nats.MemoryStorage,
	})
	require.NoError(t, err)

	cfg := NatsJetStreamConsumerConfig{
		URL:                 url,
		StreamName:          streamName,
		Subjects:            []string{subject},
		DurableConsumerName: durableConsumerName,
		DeliverPolicy:       "new",
		MethodHeader:        "test-method",
	}

	publish := func(data string) {
		msg := nats.NewMsg(subject)
		msg.Data = []byte(data)
		msg.Header.Set("test-method", "publish")
		_, err := js.PublishMsg(msg)
		require.NoError(t, err)
	}

	// requireAcked waits until the server state of the durable has every
	// delivered message acknowledged up to the stream sequence seq.
	requireAcked := func(seq uint64) {
		require.Eventually(t, func() bool {
			info, err := js.ConsumerInfo(streamName, durableConsumerName)
			return err == nil && info.NumAckPending == 0 && info.NumRedelivered == 0 &&
				info.AckFloor.Stream == seq
		}, 5*time.Second, 10*time.Millisecond, "messages delivered but not acknowledged")
	}

	dispatching := make(chan struct{})
	release := make(chan struct{})
	consumer, err := NewNatsJetStreamConsumer(cfg, &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			close(dispatching)
			<-release
			return nil
		},
	}, testCommon(prometheus.NewRegistry()))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- consumer.Run(ctx) }()

	publish("first")

	waitCh(t, dispatching, 10*time.Second, "timeout waiting for dispatch")
	cancel()
	closeDone := make(chan error, 1)
	go func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		closeDone <- consumer.Close(closeCtx)
	}()
	close(release)
	require.NoError(t, <-closeDone)
	require.ErrorIs(t, <-runDone, context.Canceled)

	// Not acknowledged, the first message would stay pending on the server
	// for AckWait (30s) and then be redelivered.
	requireAcked(1)

	markerReceived := make(chan struct{})
	var redelivered atomic.Int32
	consumer2, err := NewNatsJetStreamConsumer(cfg, &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			if string(data) == "marker" {
				close(markerReceived)
			} else {
				redelivered.Add(1)
			}
			return nil
		},
	}, testCommon(prometheus.NewRegistry()))
	require.NoError(t, err)
	ctx2, cancel2 := context.WithCancel(context.Background())
	run2Done := make(chan error, 1)
	go func() { run2Done <- consumer2.Run(ctx2) }()

	publish("marker")
	waitCh(t, markerReceived, 10*time.Second, "timeout waiting for marker")
	require.Zero(t, redelivered.Load(), "message acknowledged on shutdown was delivered again")

	cancel2()
	require.NoError(t, consumer2.Close(context.Background()))
	require.ErrorIs(t, <-run2Done, context.Canceled)
	requireAcked(2)
}
