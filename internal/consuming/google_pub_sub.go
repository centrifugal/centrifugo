package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/logging"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
)

type GooglePubSubConsumerConfig = configtypes.GooglePubSubConsumerConfig

// GooglePubSubConsumer represents a Google Pub/Sub consumer.
type GooglePubSubConsumer struct {
	config     GooglePubSubConsumerConfig
	dispatcher Dispatcher
	client     *pubsub.Client
	common     *consumerCommon
}

// NewGooglePubSubConsumer creates a new Google Pub/Sub consumer with the provided auth mechanism.
func NewGooglePubSubConsumer(
	config GooglePubSubConsumerConfig, dispatcher Dispatcher, common *consumerCommon,
) (*GooglePubSubConsumer, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := pubsub.NewClient(ctx, config.ProjectID, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	consumer := &GooglePubSubConsumer{
		config:     config,
		dispatcher: dispatcher,
		client:     client,
		common:     common,
	}
	return consumer, nil
}

// Run starts the consumer and blocks until the provided context is canceled or an error occurs.
func (c *GooglePubSubConsumer) Run(ctx context.Context) error {
	defer func() {
		_ = c.client.Close()
	}()

	// For each subscription in the configuration, spawn a separate goroutine.
	var wg sync.WaitGroup
	for _, subID := range c.config.Subscriptions {
		wg.Add(1)
		go func(subscriptionID string) {
			defer wg.Done()

			sub := c.client.Subscription(subscriptionID)
			sub.ReceiveSettings.MaxOutstandingMessages = c.config.MaxOutstandingMessages
			sub.ReceiveSettings.MaxOutstandingBytes = c.config.MaxOutstandingBytes
			sub.ReceiveSettings.NumGoroutines = 10

			err := sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
				if logging.Enabled(logging.DebugLevel) {
					c.common.log.Debug().Str("subscription", subID).
						Msg("received message from subscription")
				}
				c.dispatchMessage(ctx, msg)
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				c.common.log.Error().Err(err).Msgf("error receiving messages for subscription %s", subscriptionID)
			}
		}(subID)
	}

	wg.Wait()
	return ctx.Err()
}

func (c *GooglePubSubConsumer) dispatchMessage(ctx context.Context, msg *pubsub.Message) {
	c.processSingleMessage(ctx, msg)
}

func (c *GooglePubSubConsumer) processSingleMessage(ctx context.Context, msg *pubsub.Message) {
	var err error
	if c.config.PublicationDataMode.Enabled {
		err = c.processPublicationDataMessage(ctx, msg)
	} else {
		err = c.processCommandMessage(ctx, msg)
	}
	if err == nil {
		msg.Ack()
		c.common.metrics.processedTotal.WithLabelValues(c.common.name).Inc()
		return
	}
	msg.Nack()
	c.common.metrics.errorsTotal.WithLabelValues(c.common.name).Inc()
}

// processPublicationDataMessage handles messages in publication data mode.
func (c *GooglePubSubConsumer) processPublicationDataMessage(ctx context.Context, msg *pubsub.Message) error {
	data := msg.Data
	idempotencyKey := getAttributeValue(msg, c.config.PublicationDataMode.IdempotencyKeyAttribute)
	delta, err := getGoogleBoolAttributeValue(msg, c.config.PublicationDataMode.DeltaAttribute)
	if err != nil {
		c.common.log.Error().Err(err).Msg("error parsing delta attribute, skipping message")
		return nil // Skip message on parsing error.
	}
	channelsAttr := getAttributeValue(msg, c.config.PublicationDataMode.ChannelsAttribute)
	channels := strings.Split(channelsAttr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		c.common.log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := publicationTagsFromAttributes(msg, c.config.PublicationDataMode.TagsAttributePrefix)
	version, err := getGoogleUint64AttributeValue(msg, c.config.PublicationDataMode.VersionAttribute)
	if err != nil {
		c.common.log.Error().Err(err).Msg("error parsing version attribute, skipping message")
		return nil // Skip message on parsing error.
	}
	return c.dispatcher.DispatchPublication(ctx, channels, api.ConsumedPublication{
		Data:           data,
		IdempotencyKey: idempotencyKey,
		Delta:          delta,
		Tags:           tags,
		Version:        version,
		VersionEpoch:   getAttributeValue(msg, c.config.PublicationDataMode.VersionEpochAttribute),
	})
}

// processCommandMessage handles non-publication messages.
func (c *GooglePubSubConsumer) processCommandMessage(ctx context.Context, msg *pubsub.Message) error {
	method := getAttributeValue(msg, c.config.MethodAttribute)
	return c.dispatcher.DispatchCommand(ctx, method, msg.Data)
}

func getGoogleUint64AttributeValue(msg *pubsub.Message, key string) (uint64, error) {
	if key == "" {
		return 0, nil
	}
	val := msg.Attributes[key]
	if val == "" {
		return 0, nil
	}
	uintVal, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing attribute %s: %w", key, err)
	}
	return uintVal, nil
}

func getGoogleBoolAttributeValue(msg *pubsub.Message, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	val := msg.Attributes[key]
	if val == "" {
		return false, nil
	}
	return strconv.ParseBool(val)
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
	var tags map[string]string
	for k, v := range msg.Attributes {
		if strings.HasPrefix(k, prefix) {
			if tags == nil {
				tags = make(map[string]string)
			}
			tags[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return tags
}
