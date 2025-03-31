package consuming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/logging"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	endpoints "github.com/aws/smithy-go/endpoints"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type AwsSqsConsumerConfig = configtypes.AwsSqsConsumerConfig

// AwsSqsConsumer consumes messages from AWS SQS (or SNS delivered to SQS).
// It uses per-group ordering when enabled and supports both command and publication data modes.
type AwsSqsConsumer struct {
	name       string
	config     AwsSqsConsumerConfig
	dispatcher Dispatcher
	client     *sqs.Client
	metrics    *commonMetrics
	log        zerolog.Logger
}

type overrideEndpointResolver struct {
	Endpoint endpoints.Endpoint
}

func (o overrideEndpointResolver) ResolveEndpoint(_ context.Context, _ sqs.EndpointParameters) (endpoints.Endpoint, error) {
	return o.Endpoint, nil
}

// NewAwsSqsConsumer creates and initializes a new AwsSqsConsumer.
func NewAwsSqsConsumer(
	name string, cfg AwsSqsConsumerConfig, dispatcher Dispatcher, metrics *commonMetrics,
) (*AwsSqsConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}
	if cfg.CredentialsProfile != "" {
		loadOpts = append(loadOpts, config.WithSharedConfigProfile(cfg.CredentialsProfile))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var sqsOpts []func(*sqs.Options)
	var client *sqs.Client
	if cfg.LocalStackEndpoint != "" {
		parsedURL, err := url.Parse(cfg.LocalStackEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to parse LocalStack endpoint: %w", err)
		}
		sqsOpts = append(sqsOpts,
			sqs.WithEndpointResolverV2(overrideEndpointResolver{
				Endpoint: endpoints.Endpoint{
					URI: *parsedURL,
				},
			}),
		)
	}

	// If AssumeRoleARN is provided, assume that role.
	if cfg.AssumeRoleARN != "" {
		stsClient := sts.NewFromConfig(awsCfg)
		assumeRoleProvider := stscreds.NewAssumeRoleProvider(stsClient, cfg.AssumeRoleARN)
		awsCfg.Credentials = aws.NewCredentialsCache(assumeRoleProvider)
	}

	client = sqs.NewFromConfig(awsCfg, sqsOpts...)

	consumer := &AwsSqsConsumer{
		name:       name,
		config:     cfg,
		dispatcher: dispatcher,
		client:     client,
		log:        log.With().Str("consumer", name).Logger(),
		metrics:    metrics,
	}
	return consumer, nil
}

// Run is the main loop that receives messages from each queue and processes them in batches.
func (c *AwsSqsConsumer) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	maxConcurrency := c.config.MaxConcurrency
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}

	// Spawn one goroutine per queue URL.
	for _, queueURL := range c.config.Queues {
		wg.Add(1)
		go func(qURL string) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				waitTimeSeconds := int32(c.config.PollWaitTime.ToDuration().Seconds())
				if waitTimeSeconds < 1 {
					waitTimeSeconds = 1
				}

				visibilityTimeoutSeconds := int32(c.config.VisibilityTimeout.ToDuration().Seconds())
				if visibilityTimeoutSeconds < 1 {
					visibilityTimeoutSeconds = 1
				}

				out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(qURL),
					MaxNumberOfMessages:         c.config.MaxNumberOfMessages,
					WaitTimeSeconds:             waitTimeSeconds,
					VisibilityTimeout:           visibilityTimeoutSeconds,
					MessageAttributeNames:       []string{"All"},
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"All"},
				})
				if err != nil {
					c.log.Error().Err(err).Msgf("failed to receive messages from queue %s", qURL)
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
					}
					continue
				}

				if logging.Enabled(logging.DebugLevel) {
					c.log.Debug().Str("queue", qURL).Int("num_messages", len(out.Messages)).
						Msg("received messages from queue")
				}

				if len(out.Messages) == 0 {
					continue
				}

				c.processMessages(ctx, out.Messages, qURL, maxConcurrency)
			}
		}(queueURL)
	}

	// Wait for context cancellation.
	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

// processMessages partitions messages into unordered and ordered groups,
// then processes them appropriately. It passes the queue URL to batch deletion.
func (c *AwsSqsConsumer) processMessages(
	ctx context.Context, messages []types.Message, queueURL string, maxConcurrency int,
) {
	// Partition messages into unordered and ordered groups.
	var unorderedMessages []types.Message
	orderedGroups := make(map[string][]types.Message, len(messages))
	for _, msg := range messages {
		groupID := ""
		if msg.Attributes != nil {
			if id, ok := msg.Attributes[string(types.MessageSystemAttributeNameMessageGroupId)]; ok && id != "" {
				groupID = id
			}
		}
		if groupID == "" {
			unorderedMessages = append(unorderedMessages, msg)
		} else {
			orderedGroups[groupID] = append(orderedGroups[groupID], msg)
		}
	}

	// Channels or maps to collect successfully processed messages.
	var (
		unorderedProcessed []types.Message
		orderedProcessed   = make(map[string][]types.Message)
	)

	var wg sync.WaitGroup
	var mu sync.Mutex

	sem := make(chan struct{}, maxConcurrency)

	// Process unordered messages concurrently.
	for _, msg := range unorderedMessages {
		wg.Add(1)
		sem <- struct{}{}
		go func(m types.Message) {
			defer wg.Done()
			defer func() { <-sem }()
			if c.processSingleMessage(ctx, m) {
				mu.Lock()
				unorderedProcessed = append(unorderedProcessed, m)
				mu.Unlock()
			}
		}(msg)
	}

	// Process each ordered group sequentially in its own goroutine.
	for groupID, groupMessages := range orderedGroups {
		wg.Add(1)
		sem <- struct{}{}
		go func(groupID string, groupMessages []types.Message) {
			defer wg.Done()
			defer func() { <-sem }()
			processedGroup := make([]types.Message, 0, len(groupMessages))
			// Process messages in the group sequentially.
			for _, m := range groupMessages {
				if c.processSingleMessage(ctx, m) {
					processedGroup = append(processedGroup, m)
				} else {
					// Abort further processing in this group if one fails.
					break
				}
			}
			if len(processedGroup) > 0 {
				mu.Lock()
				orderedProcessed[groupID] = processedGroup
				mu.Unlock()
			}
		}(groupID, groupMessages)
	}

	wg.Wait()

	allProcessed := unorderedProcessed
	for _, groupProcessed := range orderedProcessed {
		allProcessed = append(allProcessed, groupProcessed...)
	}
	if len(allProcessed) > 0 {
		c.batchDeleteMessages(ctx, allProcessed, queueURL)
	}
}

// processSingleMessage handles processing of a single message with a maximum number of retries.
// It returns true if the message is processed successfully (so it can be deleted).
func (c *AwsSqsConsumer) processSingleMessage(ctx context.Context, msg types.Message) bool {
	data, err := c.extractMessageData(&msg)
	if err != nil {
		c.log.Error().Err(err).Msg("failed to extract message data")
		// Even on extraction errors, delete the message to prevent reprocessing.
		return true
	}

	var retries int
	var backoffDuration time.Duration
	maxRetries := 3

	for {
		var processErr error
		if c.config.PublicationDataMode.Enabled {
			processErr = c.processPublicationDataMessage(ctx, msg, data)
		} else {
			processErr = c.processCommandMessage(ctx, msg, data)
		}
		if processErr == nil {
			if retries > 0 {
				c.log.Info().Msg("message processed successfully after retries")
			}
			break
		}
		if errors.Is(processErr, context.Canceled) {
			return false
		}
		retries++
		if retries > maxRetries {
			if logging.Enabled(logging.DebugLevel) {
				c.log.Debug().Msg("max retries reached for processing message")
			}
			return false
		}
		c.log.Error().Err(processErr).Msg("error processing message, retrying")
		backoffDuration = getNextBackoffDuration(backoffDuration, retries)
		select {
		case <-time.After(backoffDuration):
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// batchDeleteMessages issues DeleteMessageBatch requests in batches of up to 10 messages.
// It logs failures from each batch deletion call.
func (c *AwsSqsConsumer) batchDeleteMessages(ctx context.Context, messages []types.Message, queueURL string) {
	const maxBatchSize = 10

	for start := 0; start < len(messages); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(messages) {
			end = len(messages)
		}

		batch := messages[start:end]
		entries := make([]types.DeleteMessageBatchRequestEntry, len(batch))
		for i, msg := range batch {
			entries[i] = types.DeleteMessageBatchRequestEntry{
				Id:            aws.String(aws.ToString(msg.MessageId)),
				ReceiptHandle: msg.ReceiptHandle,
			}
		}

		input := &sqs.DeleteMessageBatchInput{
			QueueUrl: aws.String(queueURL),
			Entries:  entries,
		}

		output, err := c.client.DeleteMessageBatch(ctx, input)
		if err != nil {
			c.log.Error().Err(err).Msg("batch deletion failed")
			continue
		}

		if len(output.Failed) > 0 {
			for _, failed := range output.Failed {
				c.log.Error().Msgf("failed to delete message ID %s: %s", aws.ToString(failed.Id), aws.ToString(failed.Message))
			}
		}
	}
}

// processPublicationDataMessage handles messages in publication data mode.
func (c *AwsSqsConsumer) processPublicationDataMessage(ctx context.Context, msg types.Message, data []byte) error {
	channelsAttr := getMessageAttributeValue(msg, c.config.PublicationDataMode.ChannelsAttribute)
	if channelsAttr == "" {
		c.log.Info().Msg("no channels found, skipping message")
		return nil
	}
	idempotencyKey := getMessageAttributeValue(msg, c.config.PublicationDataMode.IdempotencyKeyAttribute)
	var delta bool
	if deltaVal := getMessageAttributeValue(msg, c.config.PublicationDataMode.DeltaAttribute); deltaVal != "" {
		var err error
		delta, err = strconv.ParseBool(deltaVal)
		if err != nil {
			c.log.Error().Err(err).Msg("error parsing delta attribute, skipping message")
			return nil // Skip message on parsing error.
		}
	}
	channels := strings.Split(channelsAttr, ",")
	tags := publicationTagsFromMessageAttributes(msg, c.config.PublicationDataMode.TagsAttributePrefix)
	return c.dispatcher.DispatchPublication(ctx, data, idempotencyKey, delta, tags, channels...)
}

// processCommandMessage handles non-publication messages.
func (c *AwsSqsConsumer) processCommandMessage(ctx context.Context, msg types.Message, data []byte) error {
	method := getMessageAttributeValue(msg, c.config.MethodAttribute)
	return c.dispatcher.DispatchCommand(ctx, method, data)
}

// extractMessageData returns the payload to be dispatched.
// If the provider is "sns", it decodes the SNS envelope.
func (c *AwsSqsConsumer) extractMessageData(msg *types.Message) ([]byte, error) {
	if c.config.SNSEnvelope {
		var envelope struct {
			Message string `json:"Message"`
		}
		if msg.Body == nil {
			return nil, errors.New("empty message body")
		}
		if err := json.Unmarshal([]byte(*msg.Body), &envelope); err != nil {
			return nil, fmt.Errorf("failed to unmarshal SNS envelope: %w", err)
		}
		return []byte(envelope.Message), nil
	}
	if msg.Body == nil {
		return nil, errors.New("empty message body")
	}
	return []byte(*msg.Body), nil
}

// getMessageAttributeValue extracts the string value of the given attribute key from the SQS message.
func getMessageAttributeValue(msg types.Message, key string) string {
	if key == "" {
		return ""
	}
	if attr, ok := msg.MessageAttributes[key]; ok && attr.StringValue != nil {
		return *attr.StringValue
	}
	return ""
}

// publicationTagsFromMessageAttributes extracts tag values from message attributes using the given prefix.
func publicationTagsFromMessageAttributes(msg types.Message, prefix string) map[string]string {
	var tags map[string]string
	if prefix == "" {
		return tags
	}
	for k, v := range msg.MessageAttributes {
		if strings.HasPrefix(k, prefix) && v.StringValue != nil {
			if tags == nil {
				tags = make(map[string]string)
			}
			tags[strings.TrimPrefix(k, prefix)] = *v.StringValue
		}
	}
	return tags
}
