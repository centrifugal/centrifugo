package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/logging"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

type NatsJetStreamConsumerConfig = configtypes.NatsJetStreamConsumerConfig

// NatsJetStreamConsumer consumes messages from NATS JetStream.
type NatsJetStreamConsumer struct {
	name         string
	config       NatsJetStreamConsumerConfig
	dispatcher   Dispatcher
	nc           *nats.Conn
	consumer     jetstream.Consumer
	ctx          context.Context
	common       *consumerCommon
	eventHandler *natsEventHandler
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
	eventHandler := &natsEventHandler{
		log: common.log,
	}
	opts := []nats.Option{
		nats.MaxReconnects(-1),
		nats.ConnectHandler(eventHandler.connectHandler()),
		nats.ReconnectHandler(eventHandler.reconnectHandler()),
		nats.DisconnectErrHandler(eventHandler.disconnectHandler()),
	}
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

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var consumer jetstream.Consumer
	if cfg.Ordered {
		consumer, err = js.OrderedConsumer(ctx, cfg.StreamName, jetstream.OrderedConsumerConfig{
			FilterSubjects: cfg.Subjects,
			DeliverPolicy:  jetstream.DeliverNewPolicy,
		})
	} else {
		consumer, err = js.CreateOrUpdateConsumer(ctx, cfg.StreamName, jetstream.ConsumerConfig{
			FilterSubjects: cfg.Subjects,
			Durable:        cfg.DurableConsumerName,
			DeliverPolicy:  jetstream.DeliverNewPolicy,
			AckWait:        30 * time.Second,
		})
	}
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream consumer: %w", err)
	}

	return &NatsJetStreamConsumer{
		config:       cfg,
		dispatcher:   dispatcher,
		nc:           nc,
		consumer:     consumer,
		common:       common,
		eventHandler: eventHandler,
	}, nil
}

// msgHandler is the callback for incoming JetStream messages.
func (c *NatsJetStreamConsumer) msgHandler(msg jetstream.Msg) {
	if logging.Enabled(logging.DebugLevel) {
		c.common.log.Debug().Str("subject", msg.Subject()).Msg("received message from subject")
	}
	fmt.Println(string(msg.Data()))

	var processErr error

	if c.config.PublicationDataMode.Enabled {
		processErr = c.processPublicationDataMessage(msg)
	} else {
		processErr = c.processCommandMessage(msg)
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
func (c *NatsJetStreamConsumer) processPublicationDataMessage(msg jetstream.Msg) error {
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
		Data:           msg.Data(),
		IdempotencyKey: idempotencyKey,
		Delta:          delta,
		Tags:           tags,
		Version:        version,
		VersionEpoch:   getNatsHeaderValue(msg, c.config.PublicationDataMode.VersionEpochHeader),
	})
}

func (c *NatsJetStreamConsumer) processCommandMessage(msg jetstream.Msg) error {
	method := getNatsHeaderValue(msg, c.config.MethodHeader)
	return c.dispatcher.DispatchCommand(c.ctx, method, msg.Data())
}

func getNatsUint64HeaderValue(msg jetstream.Msg, key string) (uint64, error) {
	if key == "" {
		return 0, nil
	}
	val := msg.Headers().Get(key)
	if val == "" {
		return 0, nil
	}
	i, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing header %q: %w", key, err)
	}
	return i, nil
}

func getNatsBoolHeaderValue(msg jetstream.Msg, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	val := msg.Headers().Get(key)
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
func getNatsHeaderValue(msg jetstream.Msg, key string) string {
	if key == "" {
		return ""
	}
	return msg.Headers().Get(key)
}

// publicationTagsFromNatsHeaders extracts tags from message headers using the given prefix.
func publicationTagsFromNatsHeaders(msg jetstream.Msg, prefix string) map[string]string {
	var tags map[string]string
	if prefix == "" {
		return tags
	}
	for key, vals := range msg.Headers() {
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
	defer func() {
		_ = c.nc.Drain()
	}()

	cc, err := c.consumer.Consume(
		c.msgHandler,
		jetstream.ConsumeErrHandler(c.eventHandler.errorHandler()),
		jetstream.PullHeartbeat(5*time.Second),
	)
	if err != nil {
		return err
	}
	defer cc.Stop()
	// Block until context cancellation.
	<-ctx.Done()
	return ctx.Err()
}

type natsEventHandler struct {
	log zerolog.Logger
}

func (c *natsEventHandler) connectHandler() nats.ConnHandler {
	return func(conn *nats.Conn) {
		c.log.Info().Msg("connected")
	}
}

func (c *natsEventHandler) reconnectHandler() nats.ConnHandler {
	return func(conn *nats.Conn) {
		c.log.Info().Msg("reconnected")
	}
}

func (c *natsEventHandler) disconnectHandler() nats.ConnErrHandler {
	return func(conn *nats.Conn, err error) {
		c.log.Warn().Err(err).Msg("disconnected")
	}
}

func (c *natsEventHandler) errorHandler() jetstream.ConsumeErrHandlerFunc {
	return func(consumeCtx jetstream.ConsumeContext, err error) {
		if errors.Is(err, jetstream.ErrNoHeartbeat) {
			c.log.Warn().Msg("no heartbeat!!!")
		}
		c.log.Error().Err(err).Msg("error during consuming")
	}
}
