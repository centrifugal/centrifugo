//go:build integration

package pgstreambroker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/pgtest"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

var emptyEpochPublish = centrifuge.WithHistory(10, time.Minute, time.Hour)

// newEpochTestNode returns a Node which uses the broker, as recovery does.
func newEpochTestNode(t *testing.T, conf PostgresStreamBrokerConfig) (*PostgresStreamBroker, *centrifuge.Node) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	if conf.DSN == "" {
		conf.DSN = getPostgresConnString(t)
	}
	conf.NumShards = 4
	conf.BinaryData = true
	e, err := NewPostgresStreamBroker(node, conf)
	require.NoError(t, err)
	require.NoError(t, e.EnsureSchema(context.Background()))
	node.SetBroker(e)
	require.NoError(t, node.Run())
	t.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})
	return e, node
}

func forgetExpired(t *testing.T, e *PostgresStreamBroker, ch string) {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s SET expires_at = NOW() - interval '1 second' WHERE channel = $1", e.names.meta), ch)
	require.NoError(t, err)
	e.cleanupSupportTables(ctx)
}

// A position taken in an empty channel must not match a later stream: what
// was published in between is lost and the client has to be told.
func TestEmptyChannelPositionDetectsForgottenStream(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		publishAfter bool
	}{
		{"new stream started", true},
		{"channel empty again", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{})
			const ch = "forgotten"

			top, err := node.History(ch)
			require.NoError(t, err)
			since := top.StreamPosition

			for range 3 {
				_, err := node.Publish(ch, []byte("lost"), emptyEpochPublish)
				require.NoError(t, err)
			}
			forgetExpired(t, e, ch)
			if tt.publishAfter {
				_, err := node.Publish(ch, []byte("new"), emptyEpochPublish)
				require.NoError(t, err)
			}

			_, err = node.History(ch, centrifuge.WithSince(&since), centrifuge.WithLimit(centrifuge.NoLimit))
			require.ErrorIs(t, err, centrifuge.ErrorUnrecoverablePosition)
		})
	}
}

// Without any loss a position taken in an empty channel recovers everything.
func TestEmptyChannelPositionRecovers(t *testing.T) {
	t.Parallel()
	_, node := newEpochTestNode(t, PostgresStreamBrokerConfig{})
	const ch = "recovers"

	top, err := node.History(ch)
	require.NoError(t, err)
	require.NotEmpty(t, top.Epoch)
	require.Zero(t, top.Offset)
	since := top.StreamPosition

	// More than the history_size default of the meta row.
	const n = 120
	for range n {
		res, err := node.Publish(ch, []byte("x"), centrifuge.WithHistory(n, time.Minute, time.Hour))
		require.NoError(t, err)
		require.Equal(t, since.Epoch, res.Epoch)
	}
	res, err := node.History(ch, centrifuge.WithSince(&since), centrifuge.WithLimit(centrifuge.NoLimit))
	require.NoError(t, err)
	require.Len(t, res.Publications, n)
	require.Equal(t, uint64(1), res.Publications[0].Offset)
}

// Reading an empty channel keeps its epoch from expiring, and writes only
// when less than half of the TTL is left.
func TestEmptyChannelEpochRefresh(t *testing.T) {
	t.Parallel()
	e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{})
	ctx := context.Background()
	const ch = "refresh"
	read := func() string {
		res, err := node.History(ch, centrifuge.WithHistoryMetaTTL(100*time.Hour))
		require.NoError(t, err)
		return res.Epoch
	}
	rowVersion := func() string {
		var v string
		require.NoError(t, e.pool.QueryRow(ctx, fmt.Sprintf("SELECT xmin::text FROM %s WHERE channel = $1", e.names.meta), ch).Scan(&v))
		return v
	}

	epoch := read()
	require.NotEmpty(t, epoch)
	version := rowVersion()
	require.Equal(t, epoch, read())
	require.Equal(t, version, rowVersion(), "read of a fresh empty channel must not write")

	_, err := e.pool.Exec(ctx, fmt.Sprintf("UPDATE %s SET expires_at = NOW() + interval '10 hours' WHERE channel = $1", e.names.meta), ch)
	require.NoError(t, err)
	require.Equal(t, epoch, read())
	var extended bool
	require.NoError(t, e.pool.QueryRow(ctx, fmt.Sprintf("SELECT expires_at > NOW() + interval '99 hours' FROM %s WHERE channel = $1", e.names.meta), ch).Scan(&extended))
	require.True(t, extended)
}

// The epoch of an empty channel outlives a short history meta TTL: otherwise
// the first publication to an idle subscriber would be an insufficient state.
func TestEmptyChannelEpochTTLFloor(t *testing.T) {
	t.Parallel()
	e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{})
	_, err := node.History("floor", centrifuge.WithHistoryMetaTTL(time.Second))
	require.NoError(t, err)
	var ok bool
	require.NoError(t, e.pool.QueryRow(context.Background(), fmt.Sprintf(
		"SELECT expires_at > NOW() + interval '23 hours' FROM %s WHERE channel = 'floor'", e.names.meta)).Scan(&ok))
	require.True(t, ok)
}

// A replica which doesn't have the meta row yet must not hand out a position
// of its own: the primary has the epoch and the publications.
func TestEmptyChannelEpochLaggingReplica(t *testing.T) {
	// The "replica" is another schema, so it never catches up.
	replica, _, _ := newTestPostgresStreamBroker(t)
	replicaDSN := replica.conf.DSN
	t.Run("primary", func(t *testing.T) {
		e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{DSN: pgtest.SchemaDSN(t), ReplicaDSN: []string{replicaDSN}})
		require.NotSame(t, e.pool, e.getReadPool("published", true))

		pub, err := node.Publish("published", []byte("x"), emptyEpochPublish)
		require.NoError(t, err)
		res, err := node.History("published", centrifuge.WithLimit(centrifuge.NoLimit))
		require.NoError(t, err)
		require.Equal(t, pub.StreamPosition, res.StreamPosition)
		require.Len(t, res.Publications, 1)

		first, err := node.History("empty")
		require.NoError(t, err)
		require.NotEmpty(t, first.Epoch)
		second, err := node.History("empty")
		require.NoError(t, err)
		require.Equal(t, first.Epoch, second.Epoch)
	})
}

// The write of an empty channel's epoch turns off synchronous commit for its
// own transaction only.
func TestEmptyChannelEpochKeepsSynchronousCommit(t *testing.T) {
	t.Parallel()
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{DSN: getPostgresConnString(t), NumShards: 4, PoolSize: 1})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	ctx := context.Background()
	require.NoError(t, e.EnsureSchema(ctx))

	var before, after string
	require.NoError(t, e.pool.QueryRow(ctx, "SHOW synchronous_commit").Scan(&before))
	_, sp, err := e.History("commit", centrifuge.HistoryOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, sp.Epoch)
	require.NoError(t, e.pool.QueryRow(ctx, "SHOW synchronous_commit").Scan(&after))
	require.Equal(t, before, after)
}

// Cleanup must not delete a row which was refreshed after it was selected.
func TestCleanupKeepsRefreshedMeta(t *testing.T) {
	t.Parallel()
	e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{CleanupInterval: time.Hour})
	ctx := context.Background()
	const ch = "cleanup"
	pub, err := node.Publish(ch, []byte("x"), emptyEpochPublish)
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
		e.cleanupSupportTables(ctx)
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
	require.Equal(t, pub.Epoch, epoch)
}
