//go:build integration

package redisnatsbroker

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/natsbroker"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func newTestBroker(t *testing.T) *Broker {
	t.Helper()
	n, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	prefix := "centrifugo-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	nb, err := natsbroker.New(n, natsbroker.Config{NatsPrefixed: configtypes.NatsPrefixed{Prefix: prefix}})
	require.NoError(t, err)
	shard, err := centrifuge.NewRedisShard(n, centrifuge.RedisShardConfig{Address: "localhost:6379"})
	require.NoError(t, err)
	rb, err := centrifuge.NewRedisBroker(n, centrifuge.RedisBrokerConfig{
		Prefix: prefix,
		Shards: []*centrifuge.RedisShard{shard},
	})
	require.NoError(t, err)
	b, err := New(nb, rb)
	require.NoError(t, err)
	n.SetBroker(b)
	require.NoError(t, n.Run())
	// Node.Shutdown closes both brokers, but not the Redis shard they use.
	t.Cleanup(func() {
		_ = n.Shutdown(context.Background())
		shard.Close()
	})
	return b
}

func historyOpts(tags map[string]string) centrifuge.PublishOptions {
	return centrifuge.PublishOptions{
		HistorySize: 10,
		HistoryTTL:  time.Minute,
		Tags:        tags,
	}
}

// Mirrors api.Executor.Broadcast: one tags map published to many channels
// concurrently. Must not race and must not leak the internal epoch tag into
// the history of other channels.
func TestConcurrentPublishSharedTags(t *testing.T) {
	b := newTestBroker(t)
	tags := map[string]string{"k": "v"}
	const numChannels = 32
	channels := make([]string, numChannels)
	for i := range channels {
		channels[i] = "test_shared_tags_" + strconv.Itoa(i)
	}

	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(ch string) {
			defer wg.Done()
			_, err := b.Publish(ch, []byte(`{}`), historyOpts(tags))
			require.NoError(t, err)
		}(ch)
	}
	wg.Wait()

	for _, ch := range channels {
		pubs, _, err := b.History(ch, centrifuge.HistoryOptions{Filter: centrifuge.HistoryFilter{Limit: -1}})
		require.NoError(t, err)
		require.Len(t, pubs, 1)
		require.Equal(t, map[string]string{"k": "v"}, pubs[0].Tags, "channel %s", ch)
	}
}
