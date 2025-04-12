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
)

// NatsJetStreamConsumerConfig is an alias for our configuration type.
type NatsJetStreamConsumerConfig = configtypes.NatsJetStreamConsumerConfig

// NatsJetStreamConsumer consumes messages from NATS JetStream.
type NatsJetStreamConsumer struct {
	name           string
	config         NatsJetStreamConsumerConfig
	dispatcher     Dispatcher
	nc             *nats.Conn
	consumer       jetstream.Consumer
	ctx            context.Context
	common         *consumerCommon
	consumeContext jetstream.ConsumeContext
	// recreateCh signals that the consumer should be re-created (e.g. on heartbeat loss).
	recreateCh chan struct{}
}

// NewNatsJetStreamConsumer creates a new NatsJetStreamConsumer instance.
// On startup if creating a consumer fails then an error is returned.
// Later at runtime, if a heartbeat error occurs the consumer is re-created endlessly.
func NewNatsJetStreamConsumer(
	cfg NatsJetStreamConsumerConfig,
	dispatcher Dispatcher,
	common *consumerCommon,
) (*NatsJetStreamConsumer, error) {
	// Validate configuration.
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Allocate consumer instance early so that event callbacks can capture its pointer.
	c := &NatsJetStreamConsumer{
		config:     cfg,
		dispatcher: dispatcher,
		common:     common,
		recreateCh: make(chan struct{}, 1), // buffered so duplicate signals are dropped
	}

	// Build connection options using callbacks defined as closures.
	opts := []nats.Option{
		nats.MaxReconnects(-1),
		nats.ConnectHandler(func(conn *nats.Conn) {
			c.common.log.Info().Msg("connected")
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			c.common.log.Info().Msg("reconnected")
		}),
		nats.DisconnectErrHandler(func(conn *nats.Conn, err error) {
			c.common.log.Warn().Err(err).Msg("disconnected")
		}),
	}

	// Set authentication options.
	switch {
	case cfg.CredentialsFile != "":
		opts = append(opts, nats.UserCredentials(cfg.CredentialsFile))
	case cfg.Username != "":
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	case cfg.Token != "":
		opts = append(opts, nats.Token(cfg.Token))
	}

	// Setup TLS if enabled.
	if cfg.TLS.Enabled {
		tlsConfig, err := cfg.TLS.ToGoTLSConfig("nats_jetstream")
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		opts = append(opts, nats.Secure(tlsConfig))
	}

	// Create connection.
	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	c.nc = nc

	// Create the initial JetStream consumer.
	subCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	jsConsumer, err := createJetStreamConsumer(nc, cfg, subCtx)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream consumer: %w", err)
	}
	c.consumer = jsConsumer

	return c, nil
}

// createJetStreamConsumer is a helper (used both at startup and runtime)
// to create a new JetStream consumer based on the configuration.
func createJetStreamConsumer(nc *nats.Conn, cfg NatsJetStreamConsumerConfig, ctx context.Context) (jetstream.Consumer, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}
	if cfg.Ordered {
		return js.OrderedConsumer(ctx, cfg.StreamName, jetstream.OrderedConsumerConfig{
			FilterSubjects: cfg.Subjects,
			DeliverPolicy:  jetstream.DeliverNewPolicy,
		})
	}
	return js.CreateOrUpdateConsumer(ctx, cfg.StreamName, jetstream.ConsumerConfig{
		FilterSubjects: cfg.Subjects,
		Durable:        cfg.DurableConsumerName,
		DeliverPolicy:  jetstream.DeliverNewPolicy,
		AckWait:        30 * time.Second,
	})
}

// createJetStreamConsumer creates a new consumer using the instance's connection and config.
// This method is used for runtime re-creation.
func (c *NatsJetStreamConsumer) createJetStreamConsumer(ctx context.Context) (jetstream.Consumer, error) {
	return createJetStreamConsumer(c.nc, c.config, ctx)
}

// startConsume calls Consume on the current consumer.
func (c *NatsJetStreamConsumer) startConsume(ctx context.Context) error {
	consumeContext, err := c.consumer.Consume(
		c.msgHandler,
		jetstream.ConsumeErrHandler(c.errorHandler()),
		jetstream.PullHeartbeat(5*time.Second),
	)
	if err != nil {
		return err
	}
	c.consumeContext = consumeContext
	return nil
}

// triggerRecreation signals that the consumer should be re-created.
func (c *NatsJetStreamConsumer) triggerRecreation() {
	select {
	case c.recreateCh <- struct{}{}:
	default:
		// already signaled, do nothing
	}
}

// msgHandler is the callback for incoming JetStream messages.
func (c *NatsJetStreamConsumer) msgHandler(msg jetstream.Msg) {
	if logging.Enabled(logging.DebugLevel) {
		c.common.log.Debug().Str("subject", msg.Subject()).Msg("received message from subject")
	}

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

// processCommandMessage processes a command message.
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

// errorHandler returns a jetstream error handler that triggers recreation on heartbeat loss.
func (c *NatsJetStreamConsumer) errorHandler() jetstream.ConsumeErrHandlerFunc {
	return func(consumeCtx jetstream.ConsumeContext, err error) {
		if errors.Is(err, jetstream.ErrNoHeartbeat) {
			c.common.log.Warn().Msg("no heartbeat detected, triggering consumer recreation")
			c.triggerRecreation()
		}
		c.common.log.Error().Err(err).Msg("error during consuming")
	}
}

// Run starts the consumer and blocks until the context is canceled.
// If a heartbeat error occurs then the consumer is re-created
// and the consumeContext re-established with endless retries.
func (c *NatsJetStreamConsumer) Run(ctx context.Context) error {
	c.ctx = ctx
	defer func() {
		_ = c.nc.Drain()
	}()

	firstRun := true
	for {
		// For subsequent runs, attempt to recreate the consumer.
		if !firstRun {
			retries := 0
			var backoffDuration time.Duration
			for {
				recreateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				newConsumer, err := c.createJetStreamConsumer(recreateCtx)
				cancel()
				if err == nil {
					c.consumer = newConsumer
					c.common.log.Info().Msg("consumer recreation succeeded")
					break
				}
				backoffDuration = getNextBackoffDuration(backoffDuration, retries)
				retries++
				c.common.log.Error().Err(err).Str("delay", backoffDuration.String()).
					Msg("failed to recreate consumer, retrying")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoffDuration):
				}
			}
		}

		// Start the consumeContext.
		if err := c.startConsume(ctx); err != nil {
			if firstRun {
				return err // on initial startup, return error immediately
			}
			c.common.log.Error().Err(err).Msg("failed to start consumer consumeContext, retrying in 5 seconds")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		firstRun = false

		c.common.log.Info().Msg("consumer started, waiting for messages")
		// Block until context cancellation or a recreation signal is received.
		select {
		case <-ctx.Done():
			c.consumeContext.Stop()
			return ctx.Err()
		case <-c.recreateCh:
			c.common.log.Info().Msg("recreating consumer due to heartbeat error")
			c.consumeContext.Stop()
			// Loop around to recreate the consumer and re-subscribe.
		}
	}
}
