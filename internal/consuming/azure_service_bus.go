package consuming

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type AzureServiceBusConsumerConfig = configtypes.AzureServiceBusConsumerConfig

// AzureServiceBusConsumer consumes messages from Azure Service Bus.
type AzureServiceBusConsumer struct {
	name       string
	config     AzureServiceBusConsumerConfig
	dispatcher Dispatcher
	client     *azservicebus.Client
	log        zerolog.Logger
}

// NewAzureServiceBusConsumer creates a new consumer.
func NewAzureServiceBusConsumer(
	name string, cfg AzureServiceBusConsumerConfig, dispatcher Dispatcher, metrics *commonMetrics,
) (*AzureServiceBusConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var client *azservicebus.Client
	var err error
	if cfg.UseAzureIdentity {
		cred, err := azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to obtain Azure credential: %w", err)
		}
		client, err = azservicebus.NewClient(cfg.FullyQualifiedNamespace, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create Azure Service Bus client: %w", err)
		}
	} else {
		client, err = azservicebus.NewClientFromConnectionString(cfg.ConnectionString, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create client from connection string: %w", err)
		}
	}

	return &AzureServiceBusConsumer{
		name:       name,
		config:     cfg,
		dispatcher: dispatcher,
		client:     client,
		log:        log.With().Str("consumer", name).Logger(),
	}, nil
}

// Run starts the consumer until the context is canceled.
func (c *AzureServiceBusConsumer) Run(ctx context.Context) error {
	// --- SESSION MODE (native ordering) ---
	if c.config.UseSessions {
		var wg sync.WaitGroup
		// Spawn a fixed number of workers. Each worker continuously accepts the next available session.
		for i := 0; i < c.config.MaxConcurrentCalls; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					// Accept next available session.
					sr, err := c.client.AcceptNextSessionForQueue(ctx, c.config.Queue, nil)
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						c.log.Error().Err(err).Msg("error accepting next session; retrying")
						select {
						case <-ctx.Done():
							return
						case <-time.After(1 * time.Second):
						}
						continue
					}
					// Process messages for this session sequentially.
					for {
						messages, err := sr.ReceiveMessages(ctx, c.config.MaxReceiveMessages, nil)
						if err != nil {
							c.log.Error().Err(err).Msg("error receiving messages for session; closing session")
							_ = sr.Close(ctx)
							break // Accept a new session.
						}
						if len(messages) == 0 {
							_ = sr.Close(ctx)
							break
						}
						for _, msg := range messages {
							c.processMessage(ctx, msg, sr)
						}
					}
				}
			}()
		}
		wg.Wait()
		return ctx.Err()
	}

	// --- NON-SESSION MODE (unordered) ---
	// Create a separate receiver for each worker so that each one can call ReceiveMessages concurrently.
	var wg sync.WaitGroup
	for i := 0; i < c.config.MaxConcurrentCalls; i++ {
		receiver, err := c.client.NewReceiverForQueue(c.config.Queue, nil)
		if err != nil {
			return fmt.Errorf("failed to create receiver: %w", err)
		}
		wg.Add(1)
		go func(r *azservicebus.Receiver) {
			defer wg.Done()
			defer func() {
				_ = r.Close(ctx)
			}()
			for {
				messages, err := r.ReceiveMessages(ctx, c.config.MaxReceiveMessages, nil)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					c.log.Error().Err(err).Msg("error receiving messages; retrying")
					select {
					case <-time.After(1 * time.Second):
					case <-ctx.Done():
						return
					}
					continue
				}
				for _, msg := range messages {
					c.processMessage(ctx, msg, r)
				}
			}
		}(receiver)
	}
	wg.Wait()
	return ctx.Err()
}

type azureCompleter interface {
	CompleteMessage(context.Context, *azservicebus.ReceivedMessage, *azservicebus.CompleteMessageOptions) error
}

func (c *AzureServiceBusConsumer) processMessage(ctx context.Context, msg *azservicebus.ReceivedMessage, completer azureCompleter) {
	var retries int
	var backoffDuration time.Duration
	data := msg.Body

	for {
		var err error
		if c.config.PublicationDataMode.Enabled {
			err = c.processPublicationDataMessage(ctx, msg, data)
		} else {
			err = c.processCommandMessage(ctx, msg, data)
		}
		if err == nil {
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
		c.log.Error().Err(err).Msgf("error processing message, retrying in %v", backoffDuration)
		select {
		case <-time.After(backoffDuration):
		case <-ctx.Done():
			return
		}
	}

	if err := completer.CompleteMessage(ctx, msg, nil); err != nil {
		c.log.Error().Err(err).Msg("failed to complete message")
	}
}

// processPublicationDataMessage processes a message in publication data mode.
func (c *AzureServiceBusConsumer) processPublicationDataMessage(ctx context.Context, msg *azservicebus.ReceivedMessage, data []byte) error {
	idempotencyKey, _ := getProperty(msg, c.config.PublicationDataMode.IdempotencyKeyProperty)
	deltaStr, _ := getProperty(msg, c.config.PublicationDataMode.DeltaProperty)
	delta := false
	if deltaStr != "" {
		var err error
		delta, err = strconv.ParseBool(deltaStr)
		if err != nil {
			c.log.Error().Err(err).Msg("error parsing delta property, skipping message")
			return nil
		}
	}
	channelsStr, _ := getProperty(msg, c.config.PublicationDataMode.ChannelsProperty)
	channels := strings.Split(channelsStr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		c.log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := getTagsFromProperties(msg, c.config.PublicationDataMode.TagsPropertyPrefix)
	return c.dispatcher.DispatchPublication(ctx, data, idempotencyKey, delta, tags, channels...)
}

// processCommandMessage processes a message in command mode.
func (c *AzureServiceBusConsumer) processCommandMessage(ctx context.Context, msg *azservicebus.ReceivedMessage, data []byte) error {
	method, _ := getProperty(msg, c.config.MethodProperty)
	return c.dispatcher.DispatchCommand(ctx, method, data)
}

// getProperty extracts a string property from the message's ApplicationProperties.
func getProperty(msg *azservicebus.ReceivedMessage, key string) (string, bool) {
	if key == "" {
		return "", false
	}
	if val, ok := msg.ApplicationProperties[key]; ok {
		if str, ok := val.(string); ok {
			return str, true
		}
	}
	return "", false
}

// getTagsFromProperties extracts tag values from message properties with a given prefix.
func getTagsFromProperties(msg *azservicebus.ReceivedMessage, prefix string) map[string]string {
	tags := make(map[string]string)
	if prefix == "" {
		return tags
	}
	for k, v := range msg.ApplicationProperties {
		if strings.HasPrefix(k, prefix) {
			if str, ok := v.(string); ok {
				tags[strings.TrimPrefix(k, prefix)] = str
			}
		}
	}
	return tags
}
