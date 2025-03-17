package consuming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// RedisStreamsPublicationDataModeConfig holds configuration for publication data mode.
type RedisStreamsPublicationDataModeConfig struct {
	Enabled             bool   `mapstructure:"enabled" json:"enabled" yaml:"enabled" toml:"enabled"`
	ChannelsValue       string `mapstructure:"channels_value" json:"channels_value" yaml:"channels_value" toml:"channels_value"`
	IdempotencyKeyValue string `mapstructure:"idempotency_key_value" json:"idempotency_key_value" yaml:"idempotency_key_value" toml:"idempotency_key_value"`
	DeltaValue          string `mapstructure:"delta_value" json:"delta_value" yaml:"delta_value" toml:"delta_value"`
	TagsValuePrefix     string `mapstructure:"tags_value_prefix" json:"tags_value_prefix" yaml:"tags_value_prefix" toml:"tags_value_prefix"`
}

// RedisStreamsConsumerConfig holds configuration for the Redis Streams consumer.
type RedisStreamsConsumerConfig struct {
	// Redis connection settings.
	Address  string `mapstructure:"address" json:"address" yaml:"address" toml:"address"`
	Password string `mapstructure:"password" json:"password" yaml:"password" toml:"password"`
	DB       int    `mapstructure:"db" json:"db" yaml:"db" toml:"db"`

	// Stream and consumer group settings.
	StreamName    string `mapstructure:"stream_name" json:"stream_name" yaml:"stream_name" toml:"stream_name"`
	ConsumerGroup string `mapstructure:"consumer_group" json:"consumer_group" yaml:"consumer_group" toml:"consumer_group"`
	ConsumerName  string `mapstructure:"consumer_name" json:"consumer_name" yaml:"consumer_name" toml:"consumer_name"`

	// BlockTimeout is the duration to block waiting for messages.
	BlockTimeout time.Duration `mapstructure:"block_timeout" json:"block_timeout" yaml:"block_timeout" toml:"block_timeout"`

	// Ordered determines if messages should be processed sequentially (preserving order).
	// When false, messages are processed concurrently up to MaxParallelism.
	Ordered bool `mapstructure:"ordered" json:"ordered" yaml:"ordered" toml:"ordered"`
	// MaxParallelism sets the maximum number of messages processed concurrently if Ordered is false.
	MaxParallelism int `mapstructure:"max_parallelism" json:"max_parallelism" yaml:"max_parallelism" toml:"max_parallelism"`

	// PublicationDataMode configures publication data mode.
	PublicationDataMode RedisStreamsPublicationDataModeConfig `mapstructure:"publication_data_mode" json:"publication_data_mode" yaml:"publication_data_mode" toml:"publication_data_mode"`
	// MethodValue is used to extract a method for command messages.
	MethodValue string `mapstructure:"method_value" json:"method_value" yaml:"method_value" toml:"method_value"`
}

// Validate validates required fields in the config.
func (c RedisStreamsConsumerConfig) Validate() error {
	if c.Address == "" {
		return errors.New("address is required")
	}
	if c.StreamName == "" {
		return errors.New("stream_name is required")
	}
	if c.ConsumerGroup == "" {
		return errors.New("consumer_group is required")
	}
	if c.ConsumerName == "" {
		return errors.New("consumer_name is required")
	}
	if c.BlockTimeout == 0 {
		c.BlockTimeout = 5 * time.Second
	}
	if !c.Ordered && c.MaxParallelism <= 0 {
		c.MaxParallelism = 10
	}
	if c.PublicationDataMode.Enabled && c.PublicationDataMode.ChannelsValue == "" {
		return errors.New("channels_value is required when publication data mode is enabled")
	}
	return nil
}

// RedisStreamsConsumer consumes messages from a Redis Stream.
type RedisStreamsConsumer struct {
	config     RedisStreamsConsumerConfig
	dispatcher Dispatcher
	client     *redis.Client

	// semaphore used to limit concurrency when not ordered.
	semaphore chan struct{}
}

// NewRedisStreamsConsumer creates a new Redis Streams consumer.
func NewRedisStreamsConsumer(ctx context.Context, cfg RedisStreamsConsumerConfig, dispatcher Dispatcher) (*RedisStreamsConsumer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	// Create consumer group if it doesn't exist (MKSTREAM ensures the stream is created)
	err := client.XGroupCreateMkStream(ctx, cfg.StreamName, cfg.ConsumerGroup, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	consumer := &RedisStreamsConsumer{
		config:     cfg,
		dispatcher: dispatcher,
		client:     client,
	}
	if !cfg.Ordered {
		consumer.semaphore = make(chan struct{}, cfg.MaxParallelism)
	}
	return consumer, nil
}

// Run starts the consumer loop. It continuously reads messages from the stream.
func (c *RedisStreamsConsumer) Run(ctx context.Context) error {
	for {
		// XREADGROUP with BLOCK option.
		res, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.config.ConsumerGroup,
			Consumer: c.config.ConsumerName,
			Streams:  []string{c.config.StreamName, ">"},
			Count:    10,
			Block:    c.config.BlockTimeout,
		}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Error().Err(err).Msg("error reading messages; retrying")
			time.Sleep(1 * time.Second)
			continue
		}
		if len(res) == 0 {
			continue
		}
		for _, stream := range res {
			for _, msg := range stream.Messages {
				if c.config.Ordered {
					// Process sequentially.
					c.processMessage(ctx, msg)
				} else {
					// Process concurrently using semaphore.
					c.semaphore <- struct{}{}
					go func(m redis.XMessage) {
						defer func() { <-c.semaphore }()
						c.processMessage(ctx, m)
					}(msg)
				}
			}
		}
	}
}

// processMessage processes a single message with a retry loop and then acknowledges it.
func (c *RedisStreamsConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	data, err := json.Marshal(msg.Values)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal message values")
		c.ackMessage(ctx, msg)
		return
	}

	var processErr error
	if c.config.PublicationDataMode.Enabled {
		processErr = c.processPublicationDataMessage(ctx, msg, data)
	} else {
		processErr = c.processCommandMessage(ctx, msg, data)
	}

	var retries int
	var backoffDuration time.Duration
	for processErr != nil {
		if ctx.Err() != nil {
			return
		}
		retries++
		backoffDuration = getNextBackoffDuration(backoffDuration, retries)
		log.Error().Err(processErr).Msgf("error processing message, retrying in %v", backoffDuration)
		time.Sleep(backoffDuration)
		if c.config.PublicationDataMode.Enabled {
			processErr = c.processPublicationDataMessage(ctx, msg, data)
		} else {
			processErr = c.processCommandMessage(ctx, msg, data)
		}
	}
	c.ackMessage(ctx, msg)
}

// ackMessage acknowledges a processed message.
func (c *RedisStreamsConsumer) ackMessage(ctx context.Context, msg redis.XMessage) {
	err := c.client.XAck(ctx, c.config.StreamName, c.config.ConsumerGroup, msg.ID).Err()
	if err != nil {
		log.Error().Err(err).Msgf("failed to acknowledge message %s", msg.ID)
	}
}

// processPublicationDataMessage processes a message in publication data mode.
func (c *RedisStreamsConsumer) processPublicationDataMessage(ctx context.Context, msg redis.XMessage, data []byte) error {
	idempotencyKey, _ := getStringProperty(msg, c.config.PublicationDataMode.IdempotencyKeyValue)
	deltaStr, _ := getStringProperty(msg, c.config.PublicationDataMode.DeltaValue)
	delta := false
	if deltaStr != "" {
		var err error
		delta, err = strconv.ParseBool(deltaStr)
		if err != nil {
			log.Error().Err(err).Msg("error parsing delta property, skipping message")
			return nil
		}
	}
	channelsStr, _ := getStringProperty(msg, c.config.PublicationDataMode.ChannelsValue)
	channels := strings.Split(channelsStr, ",")
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		log.Info().Msg("no channels found, skipping message")
		return nil
	}
	tags := getTagsFromRedisValues(msg, c.config.PublicationDataMode.TagsValuePrefix)
	return c.dispatcher.DispatchPublication(ctx, data, idempotencyKey, delta, tags, channels...)
}

// processCommandMessage processes a message in command mode.
func (c *RedisStreamsConsumer) processCommandMessage(ctx context.Context, msg redis.XMessage, data []byte) error {
	method, _ := getStringProperty(msg, c.config.MethodValue)
	return c.dispatcher.DispatchCommand(ctx, method, data)
}

// getStringProperty extracts a string property from the message values.
func getStringProperty(msg redis.XMessage, key string) (string, bool) {
	if key == "" {
		return "", false
	}
	val, ok := msg.Values[key]
	if !ok {
		return "", false
	}
	if s, ok := val.(string); ok {
		return s, true
	}
	return fmt.Sprintf("%v", val), true
}

// getTagsFromRedisValues extracts tag values from message values with a given prefix.
func getTagsFromRedisValues(msg redis.XMessage, prefix string) map[string]string {
	tags := make(map[string]string)
	if prefix == "" {
		return tags
	}
	for k, v := range msg.Values {
		if strings.HasPrefix(k, prefix) {
			if s, ok := v.(string); ok {
				tags[strings.TrimPrefix(k, prefix)] = s
			} else {
				tags[strings.TrimPrefix(k, prefix)] = fmt.Sprintf("%v", v)
			}
		}
	}
	return tags
}
