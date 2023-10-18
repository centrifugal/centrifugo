package rule

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
	size   int
	buffer []cacheItem
	index  int32
}

type rollingCache struct {
	shards []*cacheShard
}

func newRollingCache(size int, shardCount int) *rollingCache {
	shardSize := size / shardCount
	rc := &rollingCache{
		shards: make([]*cacheShard, shardCount),
	}
	for i := range rc.shards {
		rc.shards[i] = &cacheShard{
			size:   shardSize,
			buffer: make([]cacheItem, shardSize),
		}
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
	for _, item := range shard.buffer {
		if item.channel == channel && time.Now().Before(time.Unix(0, item.expires)) {
			return item.value, true
		}
	}
	return channelOptionsResult{}, false
}

func (c *rollingCache) Set(channel string, value channelOptionsResult, ttl time.Duration) {
	shard := c.shardForKey(channel)
	index := int(atomic.AddInt32(&shard.index, 1) % int32(shard.size))
	item := cacheItem{
		channel: channel,
		value:   value,
		expires: time.Now().Add(ttl).UnixNano(),
	}
	shard.buffer[index] = item
}
