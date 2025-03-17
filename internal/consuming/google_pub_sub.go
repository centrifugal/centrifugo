package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

// GooglePubSubConsumerConfig is a configuration for the Google Pub/Sub consumer.
type GooglePubSubConsumerConfig struct {
	// Google Cloud project ID.
	ProjectID string `mapstructure:"project_id" json:"project_id" envconfig:"project_id" yaml:"project_id" toml:"project_id"`
	// SubscriptionID is the Pub/Sub subscription to consume from.
	SubscriptionID string `mapstructure:"subscription_id" json:"subscription_id" envconfig:"subscription_id" yaml:"subscription_id" toml:"subscription_id"`
	// MaxOutstandingMessages controls the maximum number of unprocessed messages.
	MaxOutstandingMessages int `mapstructure:"max_outstanding_messages" json:"max_outstanding_messages" envconfig:"max_outstanding_messages" default:"100" yaml:"max_outstanding_messages" toml:"max_outstanding_messages"`

	// MethodAttribute is an attribute name to extract a method name from the message.
	MethodAttribute string `mapstructure:"method_attribute" json:"method_attribute" envconfig:"method_attribute" yaml:"method_attribute" toml:"method_attribute"`

	// PublicationDataMode holds settings for the mode where message payload already contains data
	// ready to publish into channels.
	PublicationDataMode GooglePubSubPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" envconfig:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode"`

	// AuthMechanism specifies which authentication mechanism to use:
	// "default", "service_account", or "impersonate".
	AuthMechanism string `mapstructure:"auth_mechanism" json:"auth_mechanism" envconfig:"auth_mechanism" yaml:"auth_mechanism" toml:"auth_mechanism"`
	// CredentialsFile is the path to the service account JSON file if required.
	CredentialsFile string `mapstructure:"credentials_file" json:"credentials_file" envconfig:"credentials_file" yaml:"credentials_file" toml:"credentials_file"`

	// EnableMessageOrdering, when set to true, processes messages with a non-empty ordering key sequentially.
	EnableMessageOrdering bool `mapstructure:"enable_message_ordering" json:"enable_message_ordering" envconfig:"enable_message_ordering" yaml:"enable_message_ordering" toml:"enable_message_ordering"`
}

// Validate ensures required fields are set.
func (c GooglePubSubConsumerConfig) Validate() error {
	if c.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if c.SubscriptionID == "" {
		return errors.New("subscription_id is required")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsAttribute == "" {
		return errors.New("channels_attribute is required for publication data mode")
	}
	return nil
}

// GooglePubSubPublicationDataModeConfig is the configuration for the publication data mode.
type GooglePubSubPublicationDataModeConfig struct {
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

// GooglePubSubConsumer represents a Google Pub/Sub consumer.
type GooglePubSubConsumer struct {
	name       string
	config     GooglePubSubConsumerConfig
	dispatcher Dispatcher
	client     *pubsub.Client
	sub        *pubsub.Subscription

	// orderQueues holds channels keyed by message ordering key.
	orderQueues   map[string]chan *pubsub.Message
	orderQueuesMu sync.Mutex
}

// NewGooglePubSubConsumer creates a new Google Pub/Sub consumer with the provided auth mechanism.
func NewGooglePubSubConsumer(ctx context.Context, name string, config GooglePubSubConsumerConfig, dispatcher Dispatcher) (*GooglePubSubConsumer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	var clientOpts []option.ClientOption
	switch strings.ToLower(config.AuthMechanism) {
	case "", "default":
		// Use Application Default Credentials.
	case "service_account":
		if config.CredentialsFile == "" {
			return nil, errors.New("credentials_file must be provided for service_account auth")
		}
		clientOpts = append(clientOpts, option.WithCredentialsFile(config.CredentialsFile))
	case "impersonate":
		// Placeholder: implement impersonation logic as needed.
		if config.CredentialsFile == "" {
			return nil, errors.New("credentials_file must be provided for impersonate auth")
		}
		ts, err := getImpersonatedTokenSource(config.CredentialsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to get impersonated token source: %w", err)
		}
		clientOpts = append(clientOpts, option.WithTokenSource(ts))
	default:
		return nil, fmt.Errorf("unsupported auth mechanism: %s", config.AuthMechanism)
	}

	client, err := pubsub.NewClient(ctx, config.ProjectID, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	sub := client.Subscription(config.SubscriptionID)
	sub.ReceiveSettings.MaxOutstandingMessages = config.MaxOutstandingMessages

	consumer := &GooglePubSubConsumer{
		name:       name,
		config:     config,
		dispatcher: dispatcher,
		client:     client,
		sub:        sub,
	}
	if config.EnableMessageOrdering {
		consumer.orderQueues = make(map[string]chan *pubsub.Message)
	}
	return consumer, nil
}

// Run starts the consumer and blocks until the provided context is canceled or an error occurs.
func (c *GooglePubSubConsumer) Run(ctx context.Context) error {
	err := c.sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		c.dispatchMessage(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("error receiving messages: %w", err)
	}
	return nil
}

// dispatchMessage decides whether to process the message immediately or route it to an ordering queue.
func (c *GooglePubSubConsumer) dispatchMessage(ctx context.Context, msg *pubsub.Message) {
	if c.config.EnableMessageOrdering {
		if key := msg.OrderingKey; key != "" {
			c.orderQueuesMu.Lock()
			queue, exists := c.orderQueues[key]
			if !exists {
				queue = make(chan *pubsub.Message, 100) // Buffer size can be adjusted if needed.
				c.orderQueues[key] = queue
				go c.processOrderingQueue(ctx, key, queue)
			}
			c.orderQueuesMu.Unlock()
			select {
			case queue <- msg:
			case <-ctx.Done():
				return
			}
			return
		}
	}
	// No ordering or no ordering key; process immediately.
	c.processSingleMessage(ctx, msg)
}

// processOrderingQueue processes messages for a specific ordering key sequentially.
func (c *GooglePubSubConsumer) processOrderingQueue(ctx context.Context, key string, queue chan *pubsub.Message) {
	for {
		select {
		case msg := <-queue:
			c.processSingleMessage(ctx, msg)
		case <-ctx.Done():
			return
		}
	}
}

// processSingleMessage processes a single message with retries (using the existing retry logic).
func (c *GooglePubSubConsumer) processSingleMessage(ctx context.Context, msg *pubsub.Message) {
	var retries int
	var backoffDuration time.Duration
	for {
		var err error
		if c.config.PublicationDataMode.Enabled {
			err = c.processPublicationDataMessage(ctx, msg)
		} else {
			err = c.processCommandMessage(ctx, msg)
		}
		if err == nil {
			msg.Ack()
			if retries > 0 {
				log.Info().Str("consumer_name", c.name).Msg("OK processing message after errors")
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		retries++
		backoffDuration = getNextBackoffDuration(backoffDuration, retries)
		log.Error().Err(err).Msgf("error processing message, retrying in %v", backoffDuration)
		select {
		case <-time.After(backoffDuration):
		case <-ctx.Done():
			return
		}
	}
}

// processPublicationDataMessage handles messages in publication data mode.
func (c *GooglePubSubConsumer) processPublicationDataMessage(ctx context.Context, msg *pubsub.Message) error {
	data := msg.Data
	idempotencyKey := getAttributeValue(msg, c.config.PublicationDataMode.IdempotencyKeyAttribute)
	delta := false
	if deltaVal := getAttributeValue(msg, c.config.PublicationDataMode.DeltaAttribute); deltaVal != "" {
		var err error
		delta, err = strconv.ParseBool(deltaVal)
		if err != nil {
			log.Error().Err(err).Msg("error parsing delta attribute, skipping message")
			return nil // Skip message on parsing error.
		}
	}
	channelsAttr := getAttributeValue(msg, c.config.PublicationDataMode.ChannelsAttribute)
	channels := strings.Split(channelsAttr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := publicationTagsFromAttributes(msg, c.config.PublicationDataMode.TagsAttributePrefix)
	return c.dispatcher.DispatchPublication(ctx, data, idempotencyKey, delta, tags, channels...)
}

// processCommandMessage handles non-publication messages.
func (c *GooglePubSubConsumer) processCommandMessage(ctx context.Context, msg *pubsub.Message) error {
	method := getAttributeValue(msg, c.config.MethodAttribute)
	return c.dispatcher.DispatchCommand(ctx, method, msg.Data)
}

// getAttributeValue extracts the value of the given attribute key from the message.
func getAttributeValue(msg *pubsub.Message, key string) string {
	if key == "" {
		return ""
	}
	return msg.Attributes[key]
}

// publicationTagsFromAttributes extracts tag values from message attributes using the given prefix.
func publicationTagsFromAttributes(msg *pubsub.Message, prefix string) map[string]string {
	if prefix == "" {
		return nil
	}
	tags := make(map[string]string)
	for k, v := range msg.Attributes {
		if strings.HasPrefix(k, prefix) {
			tags[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return tags
}

// getImpersonatedTokenSource is a placeholder for creating an OAuth2 token source for impersonation.
// Replace this with your own logic if you need to support impersonated credentials.
func getImpersonatedTokenSource(credentialsFile string) (oauth2.TokenSource, error) {
	// Implement your impersonation logic here.
	return nil, errors.New("impersonation auth mechanism is not implemented")
}
