//go:build integration

package pgstreambroker

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
	b.calls = append(b.calls, fmt.Sprintf("pub %s %d %s tags=%v client=%s delta=%v prev=%s epoch=%v",
		ch, o.Offset, data, o.Tags, client, o.UseDelta, o.PrevData, o.Epoch != ""))
	if string(data) == "fail" {
		return centrifuge.PublishResult{}, errors.New("boom")
	}
	return centrifuge.PublishResult{}, nil
}

func (b *forwardRecorder) PublishJoin(ch string, info *centrifuge.ClientInfo) error {
	b.calls = append(b.calls, "join "+ch+" "+info.ClientID)
	return nil
}

func (b *forwardRecorder) PublishLeave(ch string, info *centrifuge.ClientInfo) error {
	b.calls = append(b.calls, "leave "+ch+" "+info.ClientID)
	return nil
}

// One outbox batch reaches the fan-out broker complete and in the order of
// its rows, join and leave rows between publications included, and a failed
// publication doesn't stop the ones after it.
func TestPostgresStreamBroker_OutboxForward(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	inner := &forwardRecorder{}
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                    getPostgresConnString(t),
		NumShards:              1,
		BinaryData:             true,
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 1,
		Broker:                 inner,
		Outbox:                 OutboxConfig{BatchSize: 100},
	})
	require.NoError(t, err)
	// Workers are not started: the test processes the batch itself.
	t.Cleanup(e.pool.Close)
	require.NoError(t, e.EnsureSchema(ctx))

	publish := func(ch, data string, opts centrifuge.PublishOptions) {
		t.Helper()
		_, err := e.Publish(ch, []byte(data), opts)
		require.NoError(t, err)
	}
	// PublishJoin and PublishLeave skip Postgres with a fan-out broker, so
	// their rows are written as a node without one writes them.
	row := func(function, client string) {
		t.Helper()
		_, err := e.pool.Exec(ctx, fmt.Sprintf(`SELECT %s(p_channel => 'a', p_client_id => $1, p_user_id => 'u', p_num_shards => 1)`, function), client)
		require.NoError(t, err)
	}
	publish("a", "1", centrifuge.PublishOptions{Tags: map[string]string{"k": "v"}, ClientInfo: &centrifuge.ClientInfo{ClientID: "c", UserID: "u"}})
	publish("a", "2", centrifuge.PublishOptions{UseDelta: true})
	row(e.names.publishJoin, "j")
	publish("b", "fail", centrifuge.PublishOptions{})
	publish("a", "3", centrifuge.PublishOptions{})
	row(e.names.publishLeave, "l")
	publish("b", "4", centrifuge.PublishOptions{})

	n, _, err := e.processOutboxBatch(ctx, e.pool, 0, []int{0})
	require.NoError(t, err)
	require.Equal(t, 7, n)
	require.Equal(t, []string{
		"pub a 1 1 tags=map[k:v] client=c delta=false prev= epoch=true",
		"pub a 2 2 tags=map[] client= delta=true prev=1 epoch=true",
		"join a j",
		"pub b 1 fail tags=map[] client= delta=false prev= epoch=true",
		"pub a 3 3 tags=map[] client= delta=false prev= epoch=true",
		"leave a l",
		"pub b 2 4 tags=map[] client= delta=false prev= epoch=true",
	}, inner.calls)
}
