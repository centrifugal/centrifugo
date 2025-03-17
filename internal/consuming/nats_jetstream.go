package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// NatsJetStreamConsumerConfig holds configuration for the NATS JetStream consumer.
type NatsJetStreamConsumerConfig struct {
	// URL of the NATS server.
	URL string `mapstructure:"url" default:"nats://127.0.0.1:4222" json:"url" toml:"url" yaml:"url"`
	// Optional authentication:
	// If CredentialsFile is provided, it is used via nats.UserCredentials.
	// Otherwise, if Username is provided, Username and Password are used.
	// Alternatively, Token can be used.
	CredentialsFile string `mapstructure:"credentials_file" json:"credentials_file" toml:"credentials_file" yaml:"credentials_file"`
	Username        string `mapstructure:"username" json:"username" toml:"username" yaml:"username"`
	Password        string `mapstructure:"password" json:"password" toml:"password" yaml:"password"`
	Token           string `mapstructure:"token" json:"token" toml:"token" yaml:"token"`

	// Subjects to subscribe to.
	Subjects []string `mapstructure:"subjects" json:"subjects" toml:"subjects" yaml:"subjects"`
	// DurableConsumerName to use.
	DurableConsumerName string `mapstructure:"durable_consumer_name" json:"durable_consumer_name" toml:"durable_consumer_name" yaml:"durable_consumer_name"`

	// Ordered, when true, uses JetStream's native ordered consumer mode.
	// In this mode, the server guarantees ordered delivery; only one consumer instance
	// will actively receive messages for the durable consumer.
	Ordered bool `mapstructure:"ordered" json:"ordered" toml:"ordered" yaml:"ordered"`

	// PublicationDataMode holds settings for the mode where the message payload is ready to be published.
	PublicationDataMode NatsJetStreamPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" toml:"publication_data_mode" yaml:"publication_data_mode"`

	// MethodHeader is the header name used to extract the method for command messages.
	MethodHeader string `mapstructure:"method_header" json:"method_header" toml:"method_header" yaml:"method_header"`
}

// NatsJetStreamPublicationDataModeConfig holds settings for publication data mode.
type NatsJetStreamPublicationDataModeConfig struct {
	// Enabled enables publication data mode.
	Enabled bool `mapstructure:"enabled" json:"enabled" toml:"enabled" yaml:"enabled"`
	// ChannelsHeader is the header containing comma-separated channel names.
	ChannelsHeader string `mapstructure:"channels_header" json:"channels_header" toml:"channels_header" yaml:"channels_header"`
	// IdempotencyKeyHeader is the header for an idempotency key.
	IdempotencyKeyHeader string `mapstructure:"idempotency_key_header" json:"idempotency_key_header" toml:"idempotency_key_header" yaml:"idempotency_key_header"`
	// DeltaHeader is the header for a delta flag.
	DeltaHeader string `mapstructure:"delta_header" json:"delta_header" toml:"delta_header" yaml:"delta_header"`
	// TagsHeaderPrefix is the prefix for headers that should be treated as tags.
	TagsHeaderPrefix string `mapstructure:"tags_header_prefix" json:"tags_header_prefix" toml:"tags_header_prefix" yaml:"tags_header_prefix"`
}

// Validate validates the required fields.
func (cfg NatsJetStreamConsumerConfig) Validate() error {
	if cfg.URL == "" {
		return errors.New("url is required")
	}
	if len(cfg.Subjects) == 0 {
		return errors.New("subjects can't be empty")
	}
	if cfg.DurableConsumerName == "" {
		return errors.New("durable is required")
	}
	if cfg.PublicationDataMode.Enabled && cfg.PublicationDataMode.ChannelsHeader == "" {
		return errors.New("channels_header is required for publication data mode")
	}
	return nil
}

// NatsJetStreamConsumer consumes messages from NATS JetStream.
type NatsJetStreamConsumer struct {
	name       string
	config     NatsJetStreamConsumerConfig
	dispatcher Dispatcher
	nc         *nats.Conn
	js         nats.JetStreamContext
	subs       []*nats.Subscription
	ctx        context.Context
}

// NewNatsJetStreamConsumer creates a new NatsJetStreamConsumer.
func NewNatsJetStreamConsumer(name string, cfg NatsJetStreamConsumerConfig, dispatcher Dispatcher) (*NatsJetStreamConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Prepare NATS connection options for authentication.
	var opts []nats.Option
	if cfg.CredentialsFile != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredentialsFile))
	} else if cfg.Username != "" {
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	} else if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	}

	// Connect to NATS.
	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create a JetStream context.
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	consumer := &NatsJetStreamConsumer{
		name:       name,
		config:     cfg,
		dispatcher: dispatcher,
		nc:         nc,
		js:         js,
	}

	// Build subscription options.
	subOpts := []nats.SubOpt{
		nats.Durable(cfg.DurableConsumerName),
		nats.ManualAck(),
	}
	// Use native ordered consumer mode if enabled.
	if cfg.Ordered {
		subOpts = append(subOpts, nats.OrderedConsumer())
	}

	for _, subject := range cfg.Subjects {
		// Subscribe to the subject.
		sub, err := js.Subscribe(subject, consumer.msgHandler, subOpts...)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to subscribe: %w", err)
		}
		consumer.subs = append(consumer.subs, sub)
	}

	return consumer, nil
}

// msgHandler is the callback for incoming JetStream messages.
func (c *NatsJetStreamConsumer) msgHandler(msg *nats.Msg) {
	c.processSingleMessage(msg)
}

// processSingleMessage processes a single message with a retry loop and then acknowledges it.
func (c *NatsJetStreamConsumer) processSingleMessage(msg *nats.Msg) {
	data := msg.Data
	var processErr error
	if c.config.PublicationDataMode.Enabled {
		processErr = c.processPublicationDataMessage(msg, data)
	} else {
		processErr = c.processCommandMessage(msg, data)
	}
	if processErr == nil {
		_ = msg.Ack()
	} else {
		log.Error().Err(processErr).Msg("error processing message")
		_ = msg.Nak()
	}
}

// processPublicationDataMessage processes a message in publication data mode.
func (c *NatsJetStreamConsumer) processPublicationDataMessage(msg *nats.Msg, data []byte) error {
	idempotencyKey := getNatsHeaderValue(msg, c.config.PublicationDataMode.IdempotencyKeyHeader)
	delta := false
	if deltaVal := getNatsHeaderValue(msg, c.config.PublicationDataMode.DeltaHeader); deltaVal != "" {
		var err error
		delta, err = strconv.ParseBool(deltaVal)
		if err != nil {
			log.Error().Err(err).Msg("error parsing delta header, skipping message")
			return nil
		}
	}
	channels := strings.Split(getNatsHeaderValue(msg, c.config.PublicationDataMode.ChannelsHeader), ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := publicationTagsFromHeaders(msg, c.config.PublicationDataMode.TagsHeaderPrefix)
	return c.dispatcher.DispatchPublication(c.ctx, data, idempotencyKey, delta, tags, channels...)
}

// processCommandMessage processes a message in command mode.
func (c *NatsJetStreamConsumer) processCommandMessage(msg *nats.Msg, data []byte) error {
	method := getNatsHeaderValue(msg, c.config.MethodHeader)
	return c.dispatcher.DispatchCommand(c.ctx, method, data)
}

// getNatsHeaderValue retrieves a header value from the Nats message.
func getNatsHeaderValue(msg *nats.Msg, key string) string {
	if key == "" {
		return ""
	}
	return msg.Header.Get(key)
}

// publicationTagsFromHeaders extracts tags from message headers using the given prefix.
func publicationTagsFromHeaders(msg *nats.Msg, prefix string) map[string]string {
	tags := make(map[string]string)
	if prefix == "" {
		return tags
	}
	for key, vals := range msg.Header {
		if strings.HasPrefix(key, prefix) && len(vals) > 0 {
			tags[strings.TrimPrefix(key, prefix)] = vals[0]
		}
	}
	return tags
}

// Run starts the consumer and blocks until the context is canceled.
// When canceled, it unsubscribes and drains the connection.
func (c *NatsJetStreamConsumer) Run(ctx context.Context) error {
	// Store the context for use in message processing.
	c.ctx = ctx
	<-ctx.Done()
	for _, sub := range c.subs {
		_ = sub.Unsubscribe()
	}
	_ = c.nc.Drain()
	return ctx.Err()
}
