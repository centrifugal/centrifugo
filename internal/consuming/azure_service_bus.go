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
	"github.com/centrifugal/centrifugo/v6/internal/logging"

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
	metrics    *commonMetrics
}

// NewAzureServiceBusConsumer creates and initializes a new AzureServiceBusConsumer.
func NewAzureServiceBusConsumer(
	name string,
	cfg AzureServiceBusConsumerConfig,
	dispatcher Dispatcher,
	metrics *commonMetrics,
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
		metrics:    metrics,
	}, nil
}

// Run starts the consumer until the context is canceled.
func (c *AzureServiceBusConsumer) Run(ctx context.Context) error {
	if c.config.UseSessions {
		return c.runSessionMode(ctx)
	}
	return c.runNonSessionMode(ctx)
}

// runSessionMode consumes messages in session mode using a separate goroutine per queue worker.
func (c *AzureServiceBusConsumer) runSessionMode(ctx context.Context) error {
	var wg sync.WaitGroup

	// For each queue, spawn workers.
	for _, queue := range c.config.Queues {
		for i := 0; i < c.config.MaxConcurrentCalls; i++ {
			wg.Add(1)
			go func(queueName string) {
				defer wg.Done()
				for {
					// Respect context cancellation.
					if ctx.Err() != nil {
						return
					}

					sr, err := c.client.AcceptNextSessionForQueue(ctx, queueName, nil)
					if err != nil {
						if errors.Is(err, context.Canceled) {
							return
						}
						c.log.Error().Err(err).Msgf("failed to accept session for queue %s", queueName)
						select {
						case <-ctx.Done():
							return
						case <-time.After(time.Second):
						}
						continue
					}

					// Process the accepted session.
					c.processSession(ctx, sr, queueName)
				}
			}(queue)
		}
	}

	wg.Wait()
	return ctx.Err()
}

// processSession processes messages for a given session and renews the session lock periodically.
// The renewal interval is set to 2/3 of the remaining time until the session lock expires.
// The context passed to message processing is bound to the session lifetime.
func (c *AzureServiceBusConsumer) processSession(ctx context.Context, sr *azservicebus.SessionReceiver, queueName string) {
	// Create a session-specific context that will be canceled when the session should end.
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	// Derive a context for lock renewal from the session context.
	renewCtx, cancelRenew := context.WithCancel(sessionCtx)
	defer cancelRenew()

	// Start a goroutine to renew the session lock at 2/3 of the remaining time until expiration.
	go func() {
		for {
			select {
			case <-renewCtx.Done():
				return
			default:
			}
			// Calculate remaining time until lock expiration.
			lockUntil := sr.LockedUntil() // sr.LockedUntil() returns the time the session is locked until.
			remaining := time.Until(lockUntil)
			renewalInterval := time.Duration(float64(remaining) * (2.0 / 3.0))
			if renewalInterval <= 0 {
				c.log.Error().Msgf("session lock already expired for queue %s", queueName)
				cancelSession()
				return
			}
			// Wait for the computed renewal interval.
			select {
			case <-time.After(renewalInterval):
				// Attempt to renew the session lock.
				if err := sr.RenewSessionLock(renewCtx, nil); err != nil {
					c.log.Error().Err(err).Msgf("failed to renew session lock for queue %s", queueName)
					// On error, cancel the session to abort processing.
					cancelSession()
					return
				} else {
					c.log.Debug().Msgf("renewed session lock for queue %s", queueName)
				}
			case <-renewCtx.Done():
				return
			}
		}
	}()

	// Ensure the session is closed when processing ends.
	defer func() {
		if err := sr.Close(sessionCtx); err != nil {
			c.log.Error().Err(err).Msgf("failed to close session for queue %s", queueName)
		}
	}()

	// Process messages using the session-bound context.
	for {
		if sessionCtx.Err() != nil {
			return
		}
		messages, err := sr.ReceiveMessages(sessionCtx, c.config.MaxReceiveMessages, nil)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			c.log.Error().Err(err).Msgf("error receiving messages for session on queue %s", queueName)
			return
		}

		if logging.Enabled(logging.DebugLevel) {
			c.log.Debug().Str("queue", queueName).Int("num_messages", len(messages)).
				Msg("received messages from queue")
		}

		if len(messages) == 0 {
			// No messages – the session is ended.
			return
		}
		for _, msg := range messages {
			c.processMessage(sessionCtx, msg, sr)
		}
	}
}

// runNonSessionMode consumes messages without sessions.
func (c *AzureServiceBusConsumer) runNonSessionMode(ctx context.Context) error {
	var wg sync.WaitGroup

	// For each queue, spawn receivers.
	for _, queue := range c.config.Queues {
		for i := 0; i < c.config.MaxConcurrentCalls; i++ {
			receiver, err := c.client.NewReceiverForQueue(queue, nil)
			if err != nil {
				return fmt.Errorf("failed to create receiver for queue %s: %w", queue, err)
			}
			wg.Add(1)
			go func(r *azservicebus.Receiver, queueName string) {
				defer wg.Done()
				defer func() {
					if err := r.Close(ctx); err != nil {
						c.log.Error().Err(err).Msgf("failed to close receiver for queue %s", queueName)
					}
				}()
				for {
					if ctx.Err() != nil {
						return
					}
					messages, err := r.ReceiveMessages(ctx, c.config.MaxReceiveMessages, nil)
					if err != nil {
						if errors.Is(err, context.Canceled) {
							return
						}
						c.log.Error().Err(err).Msgf("error receiving messages from queue %s", queueName)
						select {
						case <-ctx.Done():
							return
						case <-time.After(time.Second):
						}
						continue
					}

					if logging.Enabled(logging.DebugLevel) {
						c.log.Debug().Str("queue", queueName).Int("num_messages", len(messages)).
							Msg("received messages from queue")
					}

					for _, msg := range messages {
						c.processMessage(ctx, msg, r)
					}
				}
			}(receiver, queue)
		}
	}

	wg.Wait()
	return ctx.Err()
}

// azureCompleter defines an interface for completing messages.
type azureCompleter interface {
	CompleteMessage(context.Context, *azservicebus.ReceivedMessage, *azservicebus.CompleteMessageOptions) error
}

// processMessage processes a single message with a retry mechanism.
func (c *AzureServiceBusConsumer) processMessage(ctx context.Context, msg *azservicebus.ReceivedMessage, completer azureCompleter) {
	var retries int
	var backoffDuration time.Duration
	data := msg.Body
	const maxRetries = 5

	for {
		var err error
		if c.config.PublicationDataMode.Enabled {
			err = c.processPublicationDataMessage(ctx, msg, data)
		} else {
			err = c.processCommandMessage(ctx, msg, data)
		}
		if err == nil {
			if retries > 0 {
				c.log.Info().Msg("message processed successfully after retries")
			}
			break
		}
		// Stop retrying if context is canceled or maximum attempts reached.
		if ctx.Err() != nil || retries >= maxRetries {
			c.log.Error().Err(err).Msg("max retries reached or context canceled; abandoning message")
			break
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

	// Complete the message (or log an error on failure).
	if err := completer.CompleteMessage(ctx, msg, nil); err != nil {
		c.log.Error().Err(err).Msg("failed to complete message")
		c.metrics.errorsTotal.WithLabelValues(c.name).Inc()
	} else {
		c.metrics.processedTotal.WithLabelValues(c.name).Inc()
	}
}

// processPublicationDataMessage handles messages in publication data mode.
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

// processCommandMessage handles messages in command mode.
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

// getTagsFromProperties extracts tag values from message properties using the given prefix.
func getTagsFromProperties(msg *azservicebus.ReceivedMessage, prefix string) map[string]string {
	var tags map[string]string
	if prefix == "" {
		return tags
	}
	for k, v := range msg.ApplicationProperties {
		if strings.HasPrefix(k, prefix) {
			if str, ok := v.(string); ok {
				if tags == nil {
					tags = make(map[string]string)
				}
				tags[strings.TrimPrefix(k, prefix)] = str
			}
		}
	}
	return tags
}
