package config

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRollingCache(t *testing.T) {
	size := 30
	shardCount := 1
	cache := newRollingCache(size, shardCount)

	// Testing Set.
	channel := "channel1"
	value := channelOptionsResult{nsName: "value1"}
	ttl := 200 * time.Millisecond
	cache.Set(channel, value, ttl)

	// Testing Get after Set.
	gotValue, ok := cache.Get(channel)
	require.True(t, ok)
	require.Equal(t, value, gotValue)

	// Testing Get after expiry.
	time.Sleep(400 * time.Millisecond)
	_, ok = cache.Get(channel)
	require.False(t, ok)

	// Testing if data rolls over.
	// Setting more data than cache size, to see if rolling over works without errors.
	for i := 0; i < size+10; i++ {
		channel := "channel" + strconv.Itoa(i)
		value := channelOptionsResult{nsName: "value" + strconv.Itoa(i)}
		cache.Set(channel, value, ttl)
	}

	// The first 10 items should have been rolled over and replaced.
	// So, channel0, channel1, ... channel9 should no longer be in the cache.
	for i := 0; i < 10; i++ {
		channel := "channel" + strconv.Itoa(i)
		_, ok = cache.Get(channel)
		require.False(t, ok, channel)
	}

	// Whereas, the items from channel10 onwards should still be there.
	for i := 10; i < size+10; i++ {
		channel := "channel" + strconv.Itoa(i)
		gotValue, ok := cache.Get(channel)
		require.True(t, ok)
		require.Equal(t, channelOptionsResult{nsName: "value" + strconv.Itoa(i)}, gotValue)
	}
}
