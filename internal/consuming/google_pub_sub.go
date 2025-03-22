package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"cloud.google.com/go/pubsub"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

type GooglePubSubConsumerConfig = configtypes.GooglePubSubConsumerConfig

// GooglePubSubConsumer represents a Google Pub/Sub consumer.
type GooglePubSubConsumer struct {
	name       string
	config     GooglePubSubConsumerConfig
	dispatcher Dispatcher
	client     *pubsub.Client
	sub        *pubsub.Subscription
	metrics    *commonMetrics

	// orderQueues holds channels keyed by message ordering key.
	orderQueues   map[string]chan *pubsub.Message
	orderQueuesMu sync.Mutex

	log zerolog.Logger
}

// NewGooglePubSubConsumer creates a new Google Pub/Sub consumer with the provided auth mechanism.
func NewGooglePubSubConsumer(name string, config GooglePubSubConsumerConfig, dispatcher Dispatcher, metrics *commonMetrics) (*GooglePubSubConsumer, error) {
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
	default:
		return nil, fmt.Errorf("unsupported auth mechanism: %s", config.AuthMechanism)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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
		log:        log.With().Str("consumer", name).Logger(),
		metrics:    metrics,
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
				queue = make(chan *pubsub.Message, 100)
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

// processSingleMessage processes a single message with retry and backoff logic.
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
			c.metrics.processedTotal.WithLabelValues(c.name).Inc()
			if retries > 0 {
				c.log.Info().Msg("OK processing message after errors")
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		c.metrics.errorsTotal.WithLabelValues(c.name).Inc()
		retries++
		backoffDuration = getNextBackoffDuration(backoffDuration, retries)
		c.log.Error().Err(err).Msgf("error processing message, retrying in %v", backoffDuration)
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
			c.log.Error().Err(err).Msg("error parsing delta attribute, skipping message")
			return nil // Skip message on parsing error.
		}
	}
	channelsAttr := getAttributeValue(msg, c.config.PublicationDataMode.ChannelsAttribute)
	channels := strings.Split(channelsAttr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		c.log.Info().Msg("no channels found, skipping message")
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
