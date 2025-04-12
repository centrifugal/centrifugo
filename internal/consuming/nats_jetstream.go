package consuming

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/logging"

	"github.com/nats-io/nats.go"
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
	common     *consumerCommon
}

// NewNatsJetStreamConsumer creates a new NatsJetStreamConsumer.
func NewNatsJetStreamConsumer(
	cfg NatsJetStreamConsumerConfig,
	dispatcher Dispatcher,
	common *consumerCommon,
) (*NatsJetStreamConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Prepare NATS connection options for authentication.
	var opts []nats.Option
	switch {
	case cfg.CredentialsFile != "":
		opts = append(opts, nats.UserCredentials(cfg.CredentialsFile))
	case cfg.Username != "":
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	case cfg.Token != "":
		opts = append(opts, nats.Token(cfg.Token))
	}
	if cfg.TLS.Enabled {
		tlsConfig, err := cfg.TLS.ToGoTLSConfig("nats_jetstream")
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		opts = append(opts, nats.Secure(tlsConfig))
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
		config:     cfg,
		dispatcher: dispatcher,
		nc:         nc,
		js:         js,
		common:     common,
	}
	return consumer, nil
}

// msgHandler is the callback for incoming JetStream messages.
func (c *NatsJetStreamConsumer) msgHandler(msg *nats.Msg) {
	if logging.Enabled(logging.DebugLevel) {
		c.common.log.Debug().Str("subject", msg.Subject).
			Msg("received message from subject")
	}

	data := msg.Data
	var processErr error

	if c.config.PublicationDataMode.Enabled {
		processErr = c.processPublicationDataMessage(msg, data)
	} else {
		processErr = c.processCommandMessage(msg, data)
	}

	if processErr == nil {
		if err := msg.Ack(); err != nil {
			c.common.log.Error().Err(err).Msg("failed to ack message")
			c.common.metrics.errorsTotal.WithLabelValues(c.name).Inc()
		} else {
			c.common.metrics.processedTotal.WithLabelValues(c.name).Inc()
		}
	} else {
		c.common.log.Error().Err(processErr).Msg("processing message failed")
		if err := msg.Nak(); err != nil {
			c.common.log.Error().Err(err).Msg("failed to nak message")
		}
	}
}

// processPublicationDataMessage processes a message in publication data mode.
func (c *NatsJetStreamConsumer) processPublicationDataMessage(msg *nats.Msg, data []byte) error {
	idempotencyKey := getNatsHeaderValue(msg, c.config.PublicationDataMode.IdempotencyKeyHeader)
	delta, err := getNatsBoolHeaderValue(msg, c.config.PublicationDataMode.DeltaHeader)
	if err != nil {
		c.common.log.Error().Err(err).Msg("error parsing delta header, skipping message")
		return nil
	}
	channels := strings.Split(getNatsHeaderValue(msg, c.config.PublicationDataMode.ChannelsHeader), ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		c.common.log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := publicationTagsFromNatsHeaders(msg, c.config.PublicationDataMode.TagsHeaderPrefix)
	version, err := getNatsUint64HeaderValue(msg, c.config.PublicationDataMode.VersionHeader)
	if err != nil {
		c.common.log.Error().Err(err).Msg("error parsing version header, skipping message")
		return nil
	}
	return c.dispatcher.DispatchPublication(c.ctx, channels, api.ConsumedPublication{
		Data:           data,
		IdempotencyKey: idempotencyKey,
		Delta:          delta,
		Tags:           tags,
		Version:        version,
		VersionEpoch:   getNatsHeaderValue(msg, c.config.PublicationDataMode.VersionEpochHeader),
	})
}

// processCommandMessage processes a message in command mode.
func (c *NatsJetStreamConsumer) processCommandMessage(msg *nats.Msg, data []byte) error {
	method := getNatsHeaderValue(msg, c.config.MethodHeader)
	return c.dispatcher.DispatchCommand(c.ctx, method, data)
}

func getNatsUint64HeaderValue(msg *nats.Msg, key string) (uint64, error) {
	if key == "" {
		return 0, nil
	}
	val := msg.Header.Get(key)
	if val == "" {
		return 0, nil
	}
	i, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing header %q: %w", key, err)
	}
	return i, nil
}

func getNatsBoolHeaderValue(msg *nats.Msg, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	val := msg.Header.Get(key)
	if val == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("error parsing header %q: %w", key, err)
	}
	return b, nil
}

// getNatsHeaderValue retrieves a header value from the NATS message.
func getNatsHeaderValue(msg *nats.Msg, key string) string {
	if key == "" {
		return ""
	}
	return msg.Header.Get(key)
}

// publicationTagsFromNatsHeaders extracts tags from message headers using the given prefix.
func publicationTagsFromNatsHeaders(msg *nats.Msg, prefix string) map[string]string {
	var tags map[string]string
	if prefix == "" {
		return tags
	}
	for key, vals := range msg.Header {
		if strings.HasPrefix(key, prefix) && len(vals) > 0 {
			if tags == nil {
				tags = make(map[string]string)
			}
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
			return fmt.Errorf("failed to subscribe to subject %q: %w", subject, err)
		}
		c.subs = append(c.subs, sub)
	}

	// Block until context cancellation.
	<-ctx.Done()

	for _, sub := range c.subs {
		_ = sub.Unsubscribe()
	}
	_ = c.nc.Drain()
	return ctx.Err()
}
