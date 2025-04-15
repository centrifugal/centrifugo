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
