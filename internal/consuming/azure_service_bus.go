package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/rs/zerolog/log"
)

// AzureServiceBusPublicationDataModeConfig holds configuration for publication data mode.
type AzureServiceBusPublicationDataModeConfig struct {
	Enabled                bool   `mapstructure:"enabled" json:"enabled" yaml:"enabled" toml:"enabled"`
	ChannelsProperty       string `mapstructure:"channels_property" json:"channels_property" yaml:"channels_property" toml:"channels_property"`
	IdempotencyKeyProperty string `mapstructure:"idempotency_key_property" json:"idempotency_key_property" yaml:"idempotency_key_property" toml:"idempotency_key_property"`
	DeltaProperty          string `mapstructure:"delta_property" json:"delta_property" yaml:"delta_property" toml:"delta_property"`
	TagsPropertyPrefix     string `mapstructure:"tags_property_prefix" json:"tags_property_prefix" yaml:"tags_property_prefix" toml:"tags_property_prefix"`
}

// AzureServiceBusConsumerConfig holds configuration for the Azure Service Bus consumer.
type AzureServiceBusConsumerConfig struct {
	// For connection-string–based auth.
	ConnectionString string `mapstructure:"connection_string" json:"connection_string" yaml:"connection_string" toml:"connection_string"`
	// For Azure Identity based auth.
	FullyQualifiedNamespace string `mapstructure:"fully_qualified_namespace" json:"fully_qualified_namespace" yaml:"fully_qualified_namespace" toml:"fully_qualified_namespace"`
	TenantID                string `mapstructure:"tenant_id" json:"tenant_id" yaml:"tenant_id" toml:"tenant_id"`
	ClientID                string `mapstructure:"client_id" json:"client_id" yaml:"client_id" toml:"client_id"`
	ClientSecret            string `mapstructure:"client_secret" json:"client_secret" yaml:"client_secret" toml:"client_secret"`
	UseAzureIdentity        bool   `mapstructure:"use_azure_identity" json:"use_azure_identity" yaml:"use_azure_identity" toml:"use_azure_identity"`

	// The name of the queue or topic.
	EntityPath string `mapstructure:"entity_path" json:"entity_path" yaml:"entity_path" toml:"entity_path"`
	// If non-empty, indicates a subscription name (i.e. for topics).
	SubscriptionName string `mapstructure:"subscription_name" json:"subscription_name" yaml:"subscription_name" toml:"subscription_name"`

	// When true, the consumer uses native session-based ordering.
	// All messages must have a SessionID and Service Bus guarantees in-order delivery for each session.
	UseSessions bool `mapstructure:"use_sessions" json:"use_sessions" yaml:"use_sessions" toml:"use_sessions"`

	// MaxConcurrentCalls controls parallel processing.
	MaxConcurrentCalls int `mapstructure:"max_concurrent_calls" json:"max_concurrent_calls" yaml:"max_concurrent_calls" toml:"max_concurrent_calls"`

	// PublicationDataMode configures publication data mode.
	PublicationDataMode AzureServiceBusPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode"`
	// MethodProperty is the property name used to extract a method for command messages.
	MethodProperty string `mapstructure:"method_property" json:"method_property" yaml:"method_property" toml:"method_property"`
}

// Validate checks that required fields are set.
func (c AzureServiceBusConsumerConfig) Validate() error {
	if c.UseAzureIdentity {
		if c.FullyQualifiedNamespace == "" || c.TenantID == "" || c.ClientID == "" || c.ClientSecret == "" {
			return errors.New("when using Azure Identity, fully_qualified_namespace, tenant_id, client_id and client_secret are required")
		}
	} else {
		if c.ConnectionString == "" {
			return errors.New("connection_string is required when not using Azure Identity")
		}
	}
	if c.EntityPath == "" {
		return errors.New("entity_path is required")
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsProperty == "" {
		return errors.New("channels_property is required for publication data mode")
	}
	if c.MaxConcurrentCalls <= 0 {
		c.MaxConcurrentCalls = 1
	}
	return nil
}

// AzureServiceBusConsumer consumes messages from Azure Service Bus.
type AzureServiceBusConsumer struct {
	name       string
	config     AzureServiceBusConsumerConfig
	dispatcher Dispatcher
	client     *azservicebus.Client
}

// NewAzureServiceBusConsumer creates a new consumer.
func NewAzureServiceBusConsumer(name string, cfg AzureServiceBusConsumerConfig, dispatcher Dispatcher) (*AzureServiceBusConsumer, error) {
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
					var sr *azservicebus.SessionReceiver
					var err error
					// Accept next available session.
					if c.config.SubscriptionName != "" {
						sr, err = c.client.AcceptNextSessionForSubscription(ctx, c.config.EntityPath, c.config.SubscriptionName, nil)
					} else {
						sr, err = c.client.AcceptNextSessionForQueue(ctx, c.config.EntityPath, nil)
					}
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						log.Error().Err(err).Msg("error accepting next session; retrying")
						time.Sleep(1 * time.Second)
						continue
					}
					// Process messages for this session sequentially.
					for {
						msgs, err := sr.ReceiveMessages(ctx, 1, nil)
						if err != nil {
							log.Error().Err(err).Msg("error receiving messages for session; closing session")
							_ = sr.Close(ctx)
							break // Accept a new session.
						}
						if len(msgs) == 0 {
							_ = sr.Close(ctx)
							break
						}
						for _, msg := range msgs {
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
		var receiver *azservicebus.Receiver
		var err error
		if c.config.SubscriptionName != "" {
			receiver, err = c.client.NewReceiverForSubscription(c.config.EntityPath, c.config.SubscriptionName, nil)
		} else {
			receiver, err = c.client.NewReceiverForQueue(c.config.EntityPath, nil)
		}
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
				messages, err := r.ReceiveMessages(ctx, 10, nil)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Error().Err(err).Msg("error receiving messages; retrying")
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

func (c *AzureServiceBusConsumer) processMessage(ctx context.Context, msg *azservicebus.ReceivedMessage, receiver interface {
	CompleteMessage(context.Context, *azservicebus.ReceivedMessage, *azservicebus.CompleteMessageOptions) error
}) {
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
				log.Info().Str("consumer_name", c.name).Msg("OK processing message after errors")
			}
			break
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

	if err := receiver.CompleteMessage(ctx, msg, nil); err != nil {
		log.Error().Err(err).Msg("failed to complete message")
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
			log.Error().Err(err).Msg("error parsing delta property, skipping message")
			return nil
		}
	}
	channelsStr, _ := getProperty(msg, c.config.PublicationDataMode.ChannelsProperty)
	channels := strings.Split(channelsStr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		log.Info().Msg("no channels found, skipping message")
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
