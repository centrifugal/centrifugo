//go:build integration

package pgmapbroker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// Cleanup must not delete a meta which was refreshed after it was selected:
// the channel would get a new epoch while it has state.
func TestPostgresMapBroker_CleanupKeepsRefreshedMeta(t *testing.T) {
	t.Parallel()
	node, err := centrifuge.New(centrifuge.Config{Map: centrifuge.MapConfig{
		GetMapChannelOptions: func(string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{Mode: centrifuge.MapModeRecoverable, KeyTTL: time.Hour}
		},
	}})
	require.NoError(t, err)
	e := newTestPostgresMapBroker(t, node)
	ctx := context.Background()
	const ch = "cleanup"
	pub, err := e.Publish(ctx, ch, "key", centrifuge.MapPublishOptions{Data: []byte("x")})
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx, fmt.Sprintf("UPDATE %s SET expires_at = NOW() - interval '1 second' WHERE channel = $1", e.names.meta), ch)
	require.NoError(t, err)

	tx, err := e.pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	var pid int
	require.NoError(t, tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	_, err = tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET expires_at = NOW() + interval '1 hour' WHERE channel = $1", e.names.meta), ch)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.cleanupEntries(ctx)
	}()
	require.Eventually(t, func() bool {
		var waiting bool
		err := e.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int = ANY(pg_blocking_pids(pid)))", pid).Scan(&waiting)
		return err == nil && waiting
	}, 10*time.Second, 10*time.Millisecond, "cleanup did not reach the locked row")
	require.NoError(t, tx.Commit(ctx))
	<-done

	var epoch string
	require.NoError(t, e.pool.QueryRow(ctx, fmt.Sprintf("SELECT epoch FROM %s WHERE channel = $1", e.names.meta), ch).Scan(&epoch))
	require.Equal(t, pub.Position.Epoch, epoch)
}
