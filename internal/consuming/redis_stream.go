package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/logging"
	"github.com/centrifugal/centrifugo/v6/internal/redisqueue"
	"github.com/centrifugal/centrifugo/v6/internal/redisshard"

	"github.com/redis/rueidis"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type RedisStreamConsumerConfig = configtypes.RedisStreamConsumerConfig

// RedisStreamConsumer consumes messages from a Redis Stream.
type RedisStreamConsumer struct {
	name       string
	config     RedisStreamConsumerConfig
	dispatcher Dispatcher
	consumers  map[string]*redisqueue.Consumer
	log        zerolog.Logger
	metrics    *commonMetrics
}

// NewRedisStreamConsumer creates a new Redis Streams consumer.
func NewRedisStreamConsumer(
	name string,
	cfg RedisStreamConsumerConfig,
	dispatcher Dispatcher,
	metrics *commonMetrics,
	nodeID string,
) (*RedisStreamConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	shards, _, err := redisshard.BuildRedisShards(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("failed to build Redis shards: %w", err)
	}
	if len(shards) != 1 {
		return nil, fmt.Errorf("expected a single Redis shard, got %d", len(shards))
	}
	shard := shards[0]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if shard.RunOp(func(client rueidis.Client) rueidis.RedisResult {
		return client.Do(ctx, client.B().Ping().Build())
	}).Error() != nil {
		return nil, errors.New("failed to ping Redis")
	}

	consumer := &RedisStreamConsumer{
		name:       name,
		config:     cfg,
		dispatcher: dispatcher,
		log:        log.With().Str("consumer", name).Logger(),
		metrics:    metrics,
	}
	consumers := make(map[string]*redisqueue.Consumer)
	for _, stream := range cfg.Streams {
		streamConsumer, err := redisqueue.NewConsumer(shard, redisqueue.ConsumerOptions{
			Stream:            stream,
			ConsumerFunc:      consumer.process,
			Name:              nodeID,
			GroupName:         cfg.ConsumerGroup,
			VisibilityTimeout: cfg.VisibilityTimeout.ToDuration(),
			BlockingTimeout:   5 * time.Second,
			ReclaimInterval:   5 * time.Second,
			Concurrency:       cfg.NumMessageWorkers,
			UseLegacyReclaim:  false,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer for stream %q: %w", stream, err)
		}
		consumers[stream] = streamConsumer
	}
	consumer.consumers = consumers
	return consumer, nil
}

// process is the ConsumerFunc for redisqueue.Consumer.
func (c *RedisStreamConsumer) process(msg *redisqueue.Message) error {
	if logging.Enabled(logging.DebugLevel) {
		c.log.Debug().Str("stream", msg.ID).
			Msg("received message from stream")
	}
	dataStr, ok := msg.Values[c.config.PayloadValue]
	if !ok {
		c.log.Error().
			Str("expected_value", c.config.PayloadValue).
			Msg("payload value not found in redis stream message")
		return nil
	}

	ctx := context.Background()
	var err error
	if c.config.PublicationDataMode.Enabled {
		err = c.processPublicationDataMessage(ctx, msg, []byte(dataStr))
	} else {
		err = c.processCommandMessage(ctx, msg, []byte(dataStr))
	}
	if err != nil {
		c.metrics.errorsTotal.WithLabelValues(c.name).Inc()
		c.log.Error().Err(err).Msg("error processing redis stream message")
	} else {
		c.metrics.processedTotal.WithLabelValues(c.name).Inc()
	}
	return err
}

// Run starts the consumer loop by starting each underlying consumer in a separate goroutine.
// It also listens for context cancellation to gracefully shutdown all consumers.
func (c *RedisStreamConsumer) Run(ctx context.Context) error {
	shutdownCh := make(chan struct{})

	for stream, consumer := range c.consumers {
		errCh := make(chan error, 64)
		consumer.Errors = errCh
		go func() {
			for {
				select {
				case err := <-errCh:
					if err != nil {
						c.log.Error().Str("stream", stream).Err(err).Msg("error from consumer")
					}
				case <-shutdownCh:
					return
				}
			}
		}()

		go consumer.Run()
	}

	go func() {
		defer close(shutdownCh)
		<-ctx.Done()
		for _, consumer := range c.consumers {
			consumer.Shutdown()
		}
	}()
	<-ctx.Done()
	<-shutdownCh
	return ctx.Err()
}

// processPublicationDataMessage processes a message in publication data mode.
func (c *RedisStreamConsumer) processPublicationDataMessage(ctx context.Context, msg *redisqueue.Message, data []byte) error {
	idempotencyKey, _ := getStringProperty(msg, c.config.PublicationDataMode.IdempotencyKeyValue)
	deltaStr, _ := getStringProperty(msg, c.config.PublicationDataMode.DeltaValue)
	delta := false
	if deltaStr != "" {
		var err error
		delta, err = strconv.ParseBool(deltaStr)
		if err != nil {
			c.log.Error().Err(err).Msg("error parsing delta property, skipping message")
			return nil
		}
	}
	channelsStr, _ := getStringProperty(msg, c.config.PublicationDataMode.ChannelsValue)
	channels := strings.Split(channelsStr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		c.log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := getTagsFromRedisValues(msg, c.config.PublicationDataMode.TagsValuePrefix)
	return c.dispatcher.DispatchPublication(ctx, data, idempotencyKey, delta, tags, channels...)
}

// processCommandMessage processes a message in command mode.
func (c *RedisStreamConsumer) processCommandMessage(ctx context.Context, msg *redisqueue.Message, data []byte) error {
	method, _ := getStringProperty(msg, c.config.MethodValue)
	return c.dispatcher.DispatchCommand(ctx, method, data)
}

// getStringProperty extracts a string property from the message values.
func getStringProperty(msg *redisqueue.Message, key string) (string, bool) {
	if key == "" {
		return "", false
	}
	val, ok := msg.Values[key]
	return val, ok
}

// getTagsFromRedisValues extracts tag values from message values with the specified prefix.
func getTagsFromRedisValues(msg *redisqueue.Message, prefix string) map[string]string {
	var tags map[string]string
	if prefix == "" {
		return tags
	}
	for k, v := range msg.Values {
		if strings.HasPrefix(k, prefix) {
			if tags == nil {
				tags = make(map[string]string)
			}
			tags[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return tags
}
