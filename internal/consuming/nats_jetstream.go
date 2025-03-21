package consuming

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type NatsJetStreamConsumerConfig = configtypes.NatsJetStreamConsumerConfig

// NatsJetStreamConsumer consumes messages from NATS JetStream.
type NatsJetStreamConsumer struct {
	name       string
	config     NatsJetStreamConsumerConfig
	dispatcher Dispatcher
	nc         *nats.Conn
	js         nats.JetStreamContext
	subs       []*nats.Subscription
	ctx        context.Context
	log        zerolog.Logger
}

// NewNatsJetStreamConsumer creates a new NatsJetStreamConsumer.
func NewNatsJetStreamConsumer(
	name string, cfg NatsJetStreamConsumerConfig, dispatcher Dispatcher, metrics *commonMetrics,
) (*NatsJetStreamConsumer, error) {
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
		log:        log.With().Str("consumer", name).Logger(),
	}

	return consumer, nil
}

// msgHandler is the callback for incoming JetStream messages.
func (c *NatsJetStreamConsumer) msgHandler(msg *nats.Msg) {
	data := msg.Data
	var processErr error
	if c.config.PublicationDataMode.Enabled {
		processErr = c.processPublicationDataMessage(msg, data)
	} else {
		processErr = c.processCommandMessage(msg, data)
	}
	if processErr == nil {
		if err := msg.Ack(); err != nil {
			c.log.Error().Err(err).Msg("failed to ack message")
		}
	} else {
		c.log.Error().Err(processErr).Msg("processing message failed")
		if err := msg.Nak(); err != nil {
			c.log.Error().Err(err).Msg("failed to nak message")
		}
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
			c.log.Error().Err(err).Msg("error parsing delta header, skipping message")
			return nil
		}
	}
	channels := strings.Split(getNatsHeaderValue(msg, c.config.PublicationDataMode.ChannelsHeader), ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		c.log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := publicationTagsFromNatsHeaders(msg, c.config.PublicationDataMode.TagsHeaderPrefix)
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

// publicationTagsFromNatsHeaders extracts tags from message headers using the given prefix.
func publicationTagsFromNatsHeaders(msg *nats.Msg, prefix string) map[string]string {
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
	c.ctx = ctx
	subOpts := []nats.SubOpt{
		nats.Durable(c.config.DurableConsumerName),
		nats.ManualAck(),
		nats.DeliverLastPerSubject(),
	}
	if c.config.Ordered {
		subOpts = append(subOpts, nats.OrderedConsumer())
	}
	for _, subject := range c.config.Subjects {
		sub, err := c.js.Subscribe(subject, c.msgHandler, subOpts...)
		if err != nil {
			c.nc.Close()
			return fmt.Errorf("failed to subscribe: %w", err)
		}
		c.subs = append(c.subs, sub)
	}
	<-ctx.Done()
	for _, sub := range c.subs {
		_ = sub.Unsubscribe()
	}
	_ = c.nc.Drain()
	return ctx.Err()
}
