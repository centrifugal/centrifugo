package pubsub

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/redisshard"
)

// RedisPubSub implements PubSub using Redis PUB/SUB via redisshard.
type RedisPubSub struct {
	shards  []*redisshard.RedisShard
	channel string
}

// NewRedisPubSub creates a new Redis-backed PubSub.
// The channel name is constructed as prefix + channelName.
func NewRedisPubSub(cfg configtypes.RedisPrefixed, channelName string) (*RedisPubSub, error) {
	shards, err := redisshard.BuildRedisShards(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("error building Redis shards for pubsub: %w", err)
	}
	if len(shards) == 0 {
		return nil, fmt.Errorf("no Redis shards configured for pubsub")
	}
	return &RedisPubSub{
		shards:  shards,
		channel: cfg.Prefix + channelName,
	}, nil
}

// Publish sends data to the Redis PUB/SUB channel.
// Uses the first shard (notification channel is a single channel).
func (r *RedisPubSub) Publish(ctx context.Context, data []byte) error {
	return r.shards[0].Publish(ctx, r.channel, data)
}

// Subscribe listens on the Redis PUB/SUB channel and calls handler for each message.
// Blocks until ctx is cancelled.
func (r *RedisPubSub) Subscribe(ctx context.Context, handler func(data []byte)) error {
	return r.shards[0].Subscribe(ctx, r.channel, handler)
}

// Close releases all Redis shard connections.
func (r *RedisPubSub) Close() error {
	for _, s := range r.shards {
		s.Close()
	}
	return nil
}
