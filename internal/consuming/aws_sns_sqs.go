package consuming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type AWSConsumerConfig = configtypes.AWSConsumerConfig

// AWSConsumer consumes messages from AWS SQS (or SNS delivered to SQS).
// It uses per-group ordering when enabled and supports both command and publication data modes.
// (Assume Dispatcher and getNextBackoffDuration are defined in another file.)
type AWSConsumer struct {
	name       string
	config     AWSConsumerConfig
	dispatcher Dispatcher
	client     *sqs.Client

	// orderQueues holds channels of messages keyed by MessageGroupId.
	orderQueues   map[string]chan types.Message
	orderQueuesMu sync.Mutex

	log zerolog.Logger
}

// NewAWSConsumer creates and initializes a new AWSConsumer.
func NewAWSConsumer(
	name string, cfg AWSConsumerConfig, dispatcher Dispatcher, metrics *commonMetrics,
) (*AWSConsumer, error) {
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

	var client *sqs.Client
	if cfg.LocalStackURL != "" {
		client = sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
			o.EndpointResolver = sqs.EndpointResolverFunc(func(region string, options sqs.EndpointResolverOptions) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:               cfg.LocalStackURL,
					HostnameImmutable: true,
					SigningRegion:     "us-east-1",
				}, nil
			})
		})
	} else {
		// If AssumeRoleARN is provided, assume that role.
		if cfg.AssumeRoleARN != "" {
			stsClient := sts.NewFromConfig(awsCfg)
			assumeRoleProvider := stscreds.NewAssumeRoleProvider(stsClient, cfg.AssumeRoleARN)
			awsCfg.Credentials = aws.NewCredentialsCache(assumeRoleProvider)
		}
		client = sqs.NewFromConfig(awsCfg)
	}
	consumer := &AWSConsumer{
		name:       name,
		config:     cfg,
		dispatcher: dispatcher,
		client:     client,
		log:        log.With().Str("consumer", name).Logger(),
	}
	if cfg.EnableMessageOrdering {
		consumer.orderQueues = make(map[string]chan types.Message)
	}
	return consumer, nil
}

// Run polls SQS messages in a loop until the context is canceled.
func (c *AWSConsumer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:                    &c.config.QueueURL,
			MaxNumberOfMessages:         c.config.MaxNumberOfMessages,
			WaitTimeSeconds:             c.config.WaitTimeSeconds,
			MessageAttributeNames:       []string{"All"},
			MessageSystemAttributeNames: []types.MessageSystemAttributeName{"All"},
		})
		if err != nil {
			c.log.Error().Err(err).Msg("failed to receive messages")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
			continue
		}

		if len(out.Messages) == 0 {
			continue
		}

		for _, msg := range out.Messages {
			c.dispatchMessage(ctx, msg)
		}
	}
}

// dispatchMessage routes the message to an ordering queue if enabled and if a MessageGroupId is present.
// MessageGroupId is extracted from msg.Attributes.
func (c *AWSConsumer) dispatchMessage(ctx context.Context, msg types.Message) {
	orderKey := ""
	if c.config.EnableMessageOrdering && msg.Attributes != nil {
		if key, ok := msg.Attributes["MessageGroupId"]; ok && key != "" {
			orderKey = key
		}
	}
	if orderKey != "" {
		c.orderQueuesMu.Lock()
		queue, exists := c.orderQueues[orderKey]
		if !exists {
			queue = make(chan types.Message, 100)
			c.orderQueues[orderKey] = queue
			go c.processOrderingQueue(ctx, orderKey, queue)
		}
		c.orderQueuesMu.Unlock()
		select {
		case queue <- msg:
		case <-ctx.Done():
			return
		}
		return
	}
	c.processSingleMessage(ctx, msg)
}

// processOrderingQueue processes messages sequentially for a given MessageGroupId.
func (c *AWSConsumer) processOrderingQueue(ctx context.Context, key string, queue chan types.Message) {
	for {
		select {
		case msg := <-queue:
			c.processSingleMessage(ctx, msg)
		case <-ctx.Done():
			return
		}
	}
}

func (c *AWSConsumer) processSingleMessage(ctx context.Context, msg types.Message) {
	data, err := c.extractMessageData(&msg)
	if err != nil {
		c.log.Error().Err(err).Msg("failed to extract message data")
		c.deleteMessage(ctx, &msg)
		return
	}

	var retries int
	var backoffDuration time.Duration

	for {
		var processErr error
		if c.config.PublicationDataMode.Enabled {
			processErr = c.processPublicationDataMessage(ctx, msg, data)
		} else {
			processErr = c.processCommandMessage(ctx, msg, data)
		}
		if processErr == nil {
			if retries > 0 {
				c.log.Info().Str("consumer_name", c.name).Msg("OK processing message after errors")
			}
			break
		}
		if ctx.Err() != nil {
			return
		}
		retries++
		backoffDuration = getNextBackoffDuration(backoffDuration, retries)
		c.log.Error().Err(processErr).Msgf("error processing message, retrying in %v", backoffDuration)
		select {
		case <-time.After(backoffDuration):
		case <-ctx.Done():
			return
		}
	}
	c.deleteMessage(ctx, &msg)
}

// processPublicationDataMessage handles messages in publication data mode.
func (c *AWSConsumer) processPublicationDataMessage(ctx context.Context, msg types.Message, data []byte) error {
	idempotencyKey := getMessageAttributeValue(msg, c.config.PublicationDataMode.IdempotencyKeyAttribute)
	delta := false
	if deltaVal := getMessageAttributeValue(msg, c.config.PublicationDataMode.DeltaAttribute); deltaVal != "" {
		var err error
		delta, err = strconv.ParseBool(deltaVal)
		if err != nil {
			log.Error().Err(err).Msg("error parsing delta attribute, skipping message")
			return nil // Skip message on parsing error.
		}
	}
	channelsAttr := getMessageAttributeValue(msg, c.config.PublicationDataMode.ChannelsAttribute)
	channels := strings.Split(channelsAttr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := publicationTagsFromMessageAttributes(msg, c.config.PublicationDataMode.TagsAttributePrefix)
	return c.dispatcher.DispatchPublication(ctx, data, idempotencyKey, delta, tags, channels...)
}

// processCommandMessage handles non-publication messages.
func (c *AWSConsumer) processCommandMessage(ctx context.Context, msg types.Message, data []byte) error {
	method := getMessageAttributeValue(msg, c.config.MethodAttribute)
	return c.dispatcher.DispatchCommand(ctx, method, data)
}

// extractMessageData returns the payload to be dispatched.
// If the provider is "sns", it unmarshals the SNS envelope.
func (c *AWSConsumer) extractMessageData(msg *types.Message) ([]byte, error) {
	if strings.ToLower(c.config.Provider) == "sns" {
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
	tags := make(map[string]string)
	if prefix == "" {
		return tags
	}
	for k, v := range msg.MessageAttributes {
		if strings.HasPrefix(k, prefix) && v.StringValue != nil {
			tags[strings.TrimPrefix(k, prefix)] = *v.StringValue
		}
	}
	return tags
}

// deleteMessage removes the message from the SQS queue.
func (c *AWSConsumer) deleteMessage(ctx context.Context, msg *types.Message) {
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &c.config.QueueURL,
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		c.log.Error().Err(err).Msg("failed to delete message")
	}
}
