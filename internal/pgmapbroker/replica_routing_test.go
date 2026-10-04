//go:build integration

package pgmapbroker

import (
	"context"
	"fmt"
	"testing"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// Reads of a channel go to the replica which the outbox worker of the
// channel's shard polls, so they see at least what was delivered live. Shards
// are assigned by PostgreSQL's hashtext() in SQL, which the Go routing used to
// compute with a different hash: with several replicas, reads and live
// delivery of a channel came from different replicas.
func TestReplicaRoutingFollowsSQLShards(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	dsn := getPostgresConnString(t)
	const numShards = 8
	e, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        dsn,
		NumShards:  numShards,
		ReplicaDSN: []string{dsn, dsn, dsn},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	require.Len(t, e.readPools, 3)

	ctx := context.Background()
	channels := []string{"", "chat:room", "привет", "/tenant/users/42"}
	for i := range 200 {
		channels = append(channels, fmt.Sprintf("routing:%d", i))
	}
	for _, ch := range channels {
		var shard int
		require.NoError(t, e.pool.QueryRow(ctx, "SELECT abs(hashtext($1)::bigint) % $2", ch, numShards).Scan(&shard))
		outboxPool, shards := e.outboxWorkerConfig(shard)
		require.Equal(t, []int{shard}, shards)
		require.Same(t, outboxPool, e.getReadPool(ch, true), "channel %q of shard %d", ch, shard)
	}
}
