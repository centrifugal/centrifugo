package consuming

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/redisqueue"
	"github.com/centrifugal/centrifugo/v6/internal/redisshard"

	"github.com/redis/rueidis"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type RedisStreamConsumerConfig = configtypes.RedisStreamConsumerConfig

// RedisStreamConsumer consumes messages from a Redis Stream.
type RedisStreamConsumer struct {
	config     RedisStreamConsumerConfig
	dispatcher Dispatcher
	consumers  map[string]*redisqueue.Consumer
	log        zerolog.Logger
}

// NewRedisStreamConsumer creates a new Redis Streams consumer.
func NewRedisStreamConsumer(
	name string, cfg RedisStreamConsumerConfig, dispatcher Dispatcher, metrics *commonMetrics,
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
		return nil, errors.New("expected a single Redis shard")
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
		config:     cfg,
		dispatcher: dispatcher,
		log:        log.With().Str("consumer", name).Logger(),
	}

	consumers := make(map[string]*redisqueue.Consumer)
	for _, stream := range cfg.Streams {
		streamConsumer, err := redisqueue.NewConsumer(shard, redisqueue.ConsumerOptions{
			Stream:            stream,
			ConsumerFunc:      consumer.process,
			Name:              nodeID,
			GroupName:         cfg.ConsumerGroup,
			VisibilityTimeout: 45 * time.Second,
			BlockingTimeout:   5 * time.Second,
			ReclaimInterval:   5 * time.Second,
			Concurrency:       16,
			UseLegacyReclaim:  false,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer: %w", err)
		}
		consumers[stream] = streamConsumer
	}
	consumer.consumers = consumers
	return consumer, nil
}

func (c *RedisStreamConsumer) process(msg *redisqueue.Message) error {
	dataStr, ok := msg.Values[c.config.PayloadValue]
	if !ok {
		c.log.Error().Str("expected_value", c.config.PayloadValue).Msg("payload value not found in redis stream message")
		return nil
	}
	var processErr error
	if c.config.PublicationDataMode.Enabled {
		processErr = c.processPublicationDataMessage(context.Background(), msg, []byte(dataStr))
	} else {
		processErr = c.processCommandMessage(context.Background(), msg, []byte(dataStr))
	}
	if processErr != nil {
		c.log.Error().Err(processErr).Msg("error processing redis stream message")
	}
	return processErr
}

// Run starts the consumer loop. It continuously reads messages from the stream.
func (c *RedisStreamConsumer) Run(ctx context.Context) error {
	for _, consumer := range c.consumers {
		go func() {
			consumer.Run()
		}()
	}
	go func() {
		<-ctx.Done()
		for _, consumer := range c.consumers {
			consumer.Shutdown()
		}
	}()
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

// getTagsFromRedisValues extracts tag values from message values with a given prefix.
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
