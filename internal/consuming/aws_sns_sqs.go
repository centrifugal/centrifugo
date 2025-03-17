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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/rs/zerolog/log"
)

// AWSConsumerConfig holds configuration for the AWS consumer.
type AWSConsumerConfig struct {
	// Provider should be set to "sqs" for direct SQS messages or "sns" if the queue contains SNS envelopes.
	Provider string `mapstructure:"provider" json:"provider" envconfig:"provider" yaml:"provider" toml:"provider"`
	// Region is the AWS region.
	Region string `mapstructure:"region" json:"region" envconfig:"region" yaml:"region" toml:"region"`
	// QueueURL is the URL of the SQS queue to poll.
	QueueURL string `mapstructure:"queue_url" json:"queue_url" envconfig:"queue_url" yaml:"queue_url" toml:"queue_url"`
	// MaxNumberOfMessages is the maximum number of messages to receive per poll.
	MaxNumberOfMessages int32 `mapstructure:"max_number_of_messages" json:"max_number_of_messages" envconfig:"max_number_of_messages" default:"10" yaml:"max_number_of_messages" toml:"max_number_of_messages"`
	// WaitTimeSeconds is the long-poll wait time.
	WaitTimeSeconds int32 `mapstructure:"wait_time_seconds" json:"wait_time_seconds" envconfig:"wait_time_seconds" default:"20" yaml:"wait_time_seconds" toml:"wait_time_seconds"`
	// EnableMessageOrdering, when true, processes messages with a non-empty MessageGroupId sequentially.
	EnableMessageOrdering bool `mapstructure:"enable_message_ordering" json:"enable_message_ordering" envconfig:"enable_message_ordering" yaml:"enable_message_ordering" toml:"enable_message_ordering"`

	// PublicationDataMode holds settings for the mode where message payload already contains data
	// ready to publish into channels.
	PublicationDataMode AWSPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode"`

	// Authentication options:
	// CredentialsProfile is the name of a shared credentials profile to use.
	CredentialsProfile string `mapstructure:"credentials_profile" json:"credentials_profile" envconfig:"credentials_profile" yaml:"credentials_profile" toml:"credentials_profile"`
	// AssumeRoleARN, if provided, will cause the consumer to assume the given IAM role.
	AssumeRoleARN string `mapstructure:"assume_role_arn" json:"assume_role_arn" envconfig:"assume_role_arn" yaml:"assume_role_arn" toml:"assume_role_arn"`
	// MethodAttribute is the attribute name to extract a method for command messages.
	MethodAttribute string `mapstructure:"method_attribute" json:"method_attribute" envconfig:"method_attribute" yaml:"method_attribute" toml:"method_attribute"`

	useLocalStack bool // internal flag for testing.
}

// Validate ensures required fields are set.
func (c AWSConsumerConfig) Validate() error {
	if c.Region == "" {
		return errors.New("region is required")
	}
	if c.QueueURL == "" {
		return errors.New("queue_url is required")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsAttribute == "" {
		return errors.New("channels_attribute is required for publication data mode")
	}
	return nil
}

// AWSPublicationDataModeConfig holds configuration for the publication data mode.
type AWSPublicationDataModeConfig struct {
	// Enabled enables publication data mode.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled" yaml:"enabled" toml:"enabled"`
	// ChannelsAttribute is the attribute name containing comma-separated channel names.
	ChannelsAttribute string `mapstructure:"channels_attribute" json:"channels_attribute" envconfig:"channels_attribute" yaml:"channels_attribute" toml:"channels_attribute"`
	// IdempotencyKeyAttribute is the attribute name for an idempotency key.
	IdempotencyKeyAttribute string `mapstructure:"idempotency_key_attribute" json:"idempotency_key_attribute" envconfig:"idempotency_key_attribute" yaml:"idempotency_key_attribute" toml:"idempotency_key_attribute"`
	// DeltaAttribute is the attribute name for a delta flag.
	DeltaAttribute string `mapstructure:"delta_attribute" json:"delta_attribute" envconfig:"delta_attribute" yaml:"delta_attribute" toml:"delta_attribute"`
	// TagsAttributePrefix is the prefix for attributes containing tags.
	TagsAttributePrefix string `mapstructure:"tags_attribute_prefix" json:"tags_attribute_prefix" envconfig:"tags_attribute_prefix" yaml:"tags_attribute_prefix" toml:"tags_attribute_prefix"`
}

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
}

// NewAWSConsumer creates and initializes a new AWSConsumer.
func NewAWSConsumer(ctx context.Context, name string, cfg AWSConsumerConfig, dispatcher Dispatcher) (*AWSConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Load AWS configuration.
	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}
	if cfg.CredentialsProfile != "" {
		loadOpts = append(loadOpts, config.WithSharedConfigProfile(cfg.CredentialsProfile))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// If AssumeRoleARN is provided, assume that role.
	if cfg.AssumeRoleARN != "" {
		stsClient := sts.NewFromConfig(awsCfg)
		assumeRoleProvider := stscreds.NewAssumeRoleProvider(stsClient, cfg.AssumeRoleARN)
		awsCfg.Credentials = aws.NewCredentialsCache(assumeRoleProvider)
	}

	client := sqs.NewFromConfig(awsCfg)
	if cfg.useLocalStack {
		client = sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
			o.EndpointResolver = sqs.EndpointResolverFunc(func(region string, options sqs.EndpointResolverOptions) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:               "http://localhost:4566",
					HostnameImmutable: true,
					SigningRegion:     "us-east-1",
				}, nil
			})
		})
	}

	consumer := &AWSConsumer{
		name:       name,
		config:     cfg,
		dispatcher: dispatcher,
		client:     client,
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
			QueueUrl:              &c.config.QueueURL,
			MaxNumberOfMessages:   c.config.MaxNumberOfMessages,
			WaitTimeSeconds:       c.config.WaitTimeSeconds,
			MessageAttributeNames: []string{"All"},
			AttributeNames:        []types.QueueAttributeName{"All"},
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to receive messages")
			time.Sleep(1 * time.Second)
			continue
		}

		if len(out.Messages) == 0 {
			continue
		}

		for _, msg := range out.Messages {
			// Process each message concurrently.
			go c.dispatchMessage(ctx, msg)
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
			queue = make(chan types.Message, 100) // Adjust buffer size as needed.
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
		log.Error().Err(err).Msg("failed to extract message data")
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
				log.Info().Str("consumer_name", c.name).Msg("OK processing message after errors")
			}
			break
		}
		if ctx.Err() != nil {
			return
		}
		retries++
		backoffDuration = getNextBackoffDuration(backoffDuration, retries)
		log.Error().Err(processErr).Msgf("error processing message, retrying in %v", backoffDuration)
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
		log.Error().Err(err).Msg("failed to delete message")
	}
}
