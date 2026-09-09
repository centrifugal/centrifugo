package config

import (
	"hash/fnv"
	"sync/atomic"
	"time"
)

type cacheItem struct {
	channel string
	value   channelOptionsResult
	expires int64
}

type cacheShard struct {
	// index is unsigned on purpose: it only ever grows, and unsigned overflow
	// wraps to zero cleanly. With a signed counter the wrap goes negative and
	// the modulo below yields a negative buffer index, panicking the server
	// after 2^31 Set calls land on the same shard.
	index  atomic.Uint32
	size   uint32
	buffer []atomic.Value
}

type rollingCache struct {
	shards []*cacheShard
}

func newRollingCache(shardSize int, shardCount int) *rollingCache {
	rc := &rollingCache{
		shards: make([]*cacheShard, shardCount),
	}
	for i := range rc.shards {
		shard := &cacheShard{
			size:   uint32(shardSize),
			buffer: make([]atomic.Value, shardSize),
		}
		for j := 0; j < shardSize; j++ {
			shard.buffer[j].Store(&cacheItem{}) // Initialize with zero value.
		}
		rc.shards[i] = shard
	}
	return rc
}

func (c *rollingCache) shardForKey(key string) *cacheShard {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	shardIndex := h.Sum64() % uint64(len(c.shards))
	return c.shards[shardIndex]
}

func (c *rollingCache) Get(channel string) (channelOptionsResult, bool) {
	shard := c.shardForKey(channel)
	for i := 0; i < int(shard.size); i++ {
		item := shard.buffer[i].Load().(*cacheItem)
		if item.channel == channel && time.Now().UnixNano() < item.expires {
			return item.value, true
		}
	}
	return channelOptionsResult{}, false
}

func (c *rollingCache) Set(channel string, value channelOptionsResult, ttl time.Duration) {
	shard := c.shardForKey(channel)
	index := int(shard.index.Add(1) % shard.size)
	item := &cacheItem{
		channel: channel,
		value:   value,
		expires: time.Now().Add(ttl).UnixNano(),
	}
	shard.buffer[index].Store(item)
}
