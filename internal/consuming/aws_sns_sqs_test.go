//go:build integration

package consuming

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
)

func TestAWSConsumerWithLocalStack(t *testing.T) {
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
	if err != nil {
		t.Fatalf("failed to load AWS config: %v", err)
	}
	sqsClient := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		o.EndpointResolver = sqs.EndpointResolverFunc(func(region string, options sqs.EndpointResolverOptions) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               "http://localhost:4566",
				HostnameImmutable: true,
				SigningRegion:     "us-east-1",
			}, nil
		})
	})
	// Create a queue for testing.
	queueName := "test-queue-" + uuid.NewString()
	createQueueOutput, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  &queueName,
		Attributes: map[string]string{},
	})
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}
	queueURL := *createQueueOutput.QueueUrl

	// Configure the AWS consumer.
	cfg := AWSConsumerConfig{
		Provider:              "sqs",
		Region:                "us-east-1",
		QueueURL:              queueURL,
		MaxNumberOfMessages:   10,
		WaitTimeSeconds:       2,
		EnableMessageOrdering: false,    // Set to true for FIFO queues
		MethodAttribute:       "Method", // This attribute should be in the message attributes.
		LocalStackURL:         "http://localhost:4566",
	}

	// Set up a dispatcher that signals when a message is processed.
	done := make(chan struct{})
	dispatcher := &MockDispatcher{
		onDispatchCommand: func(ctx context.Context, method string, data []byte) error {
			close(done)
			return nil
		},
	}

	consumer, err := NewAWSConsumer("test", cfg, dispatcher, newCommonMetrics(prometheus.NewRegistry()))
	if err != nil {
		t.Fatalf("failed to create AWS consumer: %v", err)
	}
	go func() {
		if err := consumer.Run(ctx); err != nil {
			t.Errorf("consumer error: %v", err)
		}
	}()

	// Publish a test message to SQS.
	// For SQS, messages are sent using SendMessage.
	// The MessageAttributes must include the method attribute.
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
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}
	waitCh(t, done, 5*time.Second, "timeout waiting for message processing")
}
