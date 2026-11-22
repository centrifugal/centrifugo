//go:build integration

package pgmapbroker

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

type forwardRecorder struct {
	centrifuge.Broker
	calls []string
}

func (b *forwardRecorder) Publish(ch string, data []byte, o centrifuge.PublishOptions) (centrifuge.PublishResult, error) {
	var client string
	if o.ClientInfo != nil {
		client = o.ClientInfo.ClientID
	}
	b.calls = append(b.calls, fmt.Sprintf("%s %d key=%s data=%s removed=%v tags=%v client=%s delta=%v prev=%s epoch=%v",
		ch, o.Offset, o.Key, data, o.Removed, o.Tags, client, o.UseDelta, o.PrevData, o.Epoch != ""))
	if string(data) == "fail" {
		return centrifuge.PublishResult{}, errors.New("boom")
	}
	return centrifuge.PublishResult{}, nil
}

// Outbox batches reach the fan-out broker complete and in the order of their
// rows, a failed publication doesn't stop the ones after it, and nothing of a
// batch is forwarded again with the next one.
func TestPostgresMapBroker_OutboxForward(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	node, err := centrifuge.New(centrifuge.Config{
		Map: centrifuge.MapConfig{
			GetMapChannelOptions: func(string) centrifuge.MapChannelOptions {
				return centrifuge.MapChannelOptions{Mode: centrifuge.MapModeRecoverable, KeyTTL: time.Minute}
			},
		},
	})
	require.NoError(t, err)
	inner := &forwardRecorder{}
	e, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        getPostgresConnString(t),
		BinaryData: true,
		NumShards:  1,
		Broker:     inner,
		Outbox:     OutboxConfig{BatchSize: 100},
	})
	require.NoError(t, err)
	// Workers are not started: the test processes the batches itself.
	t.Cleanup(e.pool.Close)
	require.NoError(t, e.EnsureSchema(ctx))

	publish := func(ch, key string, opts centrifuge.MapPublishOptions) {
		t.Helper()
		_, err := e.Publish(ctx, ch, key, opts)
		require.NoError(t, err)
	}
	publish("a", "k", centrifuge.MapPublishOptions{Data: []byte("1"), Tags: map[string]string{"t": "v"}, ClientInfo: &centrifuge.ClientInfo{ClientID: "c", UserID: "u"}})
	publish("a", "k", centrifuge.MapPublishOptions{Data: []byte("2"), UseDelta: true})
	publish("b", "x", centrifuge.MapPublishOptions{Data: []byte("fail")})
	_, err = e.Remove(ctx, "a", "k", centrifuge.MapRemoveOptions{})
	require.NoError(t, err)

	buf := &outboxBatchBuf{}
	n, cursor, err := e.processOutboxBatch(ctx, e.pool, 0, []int{0}, buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []string{
		"a 1 key=k data=1 removed=false tags=map[t:v] client=c delta=false prev= epoch=true",
		"a 2 key=k data=2 removed=false tags=map[] client= delta=true prev=1 epoch=true",
		"b 1 key=x data=fail removed=false tags=map[] client= delta=false prev= epoch=true",
		"a 3 key=k data= removed=true tags=map[] client= delta=false prev= epoch=true",
	}, inner.calls)

	inner.calls = nil
	publish("b", "y", centrifuge.MapPublishOptions{Data: []byte("5")})
	n, _, err = e.processOutboxBatch(ctx, e.pool, cursor, []int{0}, buf)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, []string{
		"b 2 key=y data=5 removed=false tags=map[] client= delta=false prev= epoch=true",
	}, inner.calls)
}
