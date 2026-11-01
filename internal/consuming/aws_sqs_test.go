//go:build integration

package consuming

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	endpoints "github.com/aws/smithy-go/endpoints"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestAWSConsumerWithLocalStack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Set dummy credentials for LocalStack.
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "test")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	_ = os.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	defer func() {
		_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
		_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		_ = os.Unsetenv("AWS_DEFAULT_REGION")
	}()

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)

	parsedURL, _ := url.Parse("http://localhost:4566")

	sqsOpts := []func(*sqs.Options){
		sqs.WithEndpointResolverV2(overrideEndpointResolver{
			Endpoint: endpoints.Endpoint{
				URI: *parsedURL,
			},
		}),
	}

	sqsClient := sqs.NewFromConfig(awsCfg, sqsOpts...)

	queueName := "test-queue-" + uuid.NewString()
	createQueueOutput, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  &queueName,
		Attributes: map[string]string{},
	})
	require.NoError(t, err)
	queueURL := *createQueueOutput.QueueUrl

	cfg := AwsSqsConsumerConfig{
		Region:              "us-east-1",
		Queues:              []string{queueURL},
		MaxNumberOfMessages: 10,
		PollWaitTime:        configtypes.Duration(2 * time.Second),
		MethodAttribute:     "Method",
		LocalStackEndpoint:  "http://localhost:4566",
	}

	var processedCount atomic.Int64

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	createConsumer := func(doneCh chan struct{}) *AwsSqsConsumer {
		dispatcher := &MockDispatcher{
			onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
				close(doneCh)
				processedCount.Add(1)
				return nil
			},
		}
		consumer, err := NewAwsSqsConsumer(cfg, dispatcher, testCommon(prometheus.NewRegistry()))
		require.NoError(t, err)
		return consumer
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	go func() {
		if err := createConsumer(done1).Run(ctx1); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer1 error: %v", err)
		}
	}()

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go func() {
		if err := createConsumer(done2).Run(ctx2); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer2 error: %v", err)
		}
	}()

	_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    &queueURL,
		MessageBody: aws.String("Test message body"),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"Method": {
				DataType:    aws.String("String"),
				StringValue: aws.String("testMethod"),
			},
		},
	})
	require.NoError(t, err)

	waitAnyCh(t, []chan struct{}{done1, done2}, 5*time.Second, "timeout waiting for message processing")
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, int64(1), processedCount.Load(), "only one consumer must receive the message")
}

// TestAwsSqsConsumer_DrainAcksInFlight verifies that a message in flight when
// the consumer is stopped is handled and deleted from the queue, and the next
// consumer of the queue does not receive it again.
func TestAwsSqsConsumer_DrainAcksInFlight(t *testing.T) {
	// Not parallel: t.Setenv sets the credentials for LocalStack in the
	// process environment.
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	ctx := context.Background()

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	require.NoError(t, err)
	parsedURL, _ := url.Parse("http://localhost:4566")
	sqsClient := sqs.NewFromConfig(awsCfg, sqs.WithEndpointResolverV2(overrideEndpointResolver{
		Endpoint: endpoints.Endpoint{URI: *parsedURL},
	}))

	queueName := "test-queue-" + uuid.NewString()
	createQueueOutput, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: &queueName,
	})
	require.NoError(t, err)
	queueURL := *createQueueOutput.QueueUrl

	// A short visibility timeout: a message left undeleted becomes visible
	// and is received again soon.
	cfg := AwsSqsConsumerConfig{
		Region:              "us-east-1",
		Queues:              []string{queueURL},
		MaxNumberOfMessages: 10,
		PollWaitTime:        configtypes.Duration(time.Second),
		VisibilityTimeout:   configtypes.Duration(time.Second),
		MethodAttribute:     "Method",
		LocalStackEndpoint:  "http://localhost:4566",
	}

	send := func(body string) {
		_, err := sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    &queueURL,
			MessageBody: aws.String(body),
			MessageAttributes: map[string]types.MessageAttributeValue{
				"Method": {DataType: aws.String("String"), StringValue: aws.String("testMethod")},
			},
		})
		require.NoError(t, err)
	}

	// queueEmpty reports whether the queue has no messages left, visible or
	// received and not deleted.
	queueEmpty := func() bool {
		out, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
			QueueUrl: &queueURL,
			AttributeNames: []types.QueueAttributeName{
				types.QueueAttributeNameApproximateNumberOfMessages,
				types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
				types.QueueAttributeNameApproximateNumberOfMessagesDelayed,
			},
		})
		if err != nil {
			return false
		}
		for _, v := range out.Attributes {
			if v != "0" {
				return false
			}
		}
		return true
	}

	dispatching := make(chan struct{})
	release := make(chan struct{})
	consumer, err := NewAwsSqsConsumer(cfg, &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			close(dispatching)
			<-release
			return nil
		},
	}, testCommon(prometheus.NewRegistry()))
	require.NoError(t, err)

	runCtx, runCancel := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() { runDone <- consumer.Run(runCtx) }()

	send("first")

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

	// The first message is deleted from the queue.
	require.Eventually(t, queueEmpty, 5*time.Second, 50*time.Millisecond, "message not deleted")

	markerReceived := make(chan struct{})
	var redelivered atomic.Int32
	consumer2, err := NewAwsSqsConsumer(cfg, &MockDispatcher{
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
	run2Ctx, run2Cancel := context.WithCancel(ctx)
	run2Done := make(chan error, 1)
	go func() { run2Done <- consumer2.Run(run2Ctx) }()

	send("marker")
	waitCh(t, markerReceived, 10*time.Second, "timeout waiting for marker")
	// The queue empties only once every message is received and deleted: an
	// undeleted first message would be received again by the consumer.
	require.Eventually(t, queueEmpty, 5*time.Second, 50*time.Millisecond, "messages not deleted")
	require.Zero(t, redelivered.Load(), "message deleted on shutdown was delivered again")

	run2Cancel()
	require.NoError(t, consumer2.Close(context.Background()))
	require.ErrorIs(t, <-run2Done, context.Canceled)
}
