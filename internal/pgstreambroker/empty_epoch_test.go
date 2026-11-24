//go:build integration

package pgstreambroker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/pgemptymeta"
	"github.com/centrifugal/centrifugo/v6/internal/pgtest"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

var emptyEpochPublish = centrifuge.WithHistory(10, time.Minute, time.Hour)

// newEpochTestNode returns a Node which uses the broker, as recovery does.
func newEpochTestNode(t *testing.T, conf PostgresStreamBrokerConfig, nodeConf ...centrifuge.Config) (*PostgresStreamBroker, *centrifuge.Node) {
	var nc centrifuge.Config
	if len(nodeConf) > 0 {
		nc = nodeConf[0]
	}
	node, err := centrifuge.New(nc)
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

func metaTTLLeft(t *testing.T, e *PostgresStreamBroker, ch string) time.Duration {
	var seconds float64
	require.NoError(t, e.pool.QueryRow(context.Background(), fmt.Sprintf(
		"SELECT EXTRACT(EPOCH FROM expires_at - NOW())::float8 FROM %s WHERE channel = $1", e.names.meta), ch).Scan(&seconds))
	return time.Duration(seconds * float64(time.Second))
}

// ageMeta makes the meta of the channels look written that long ago.
func ageMeta(t *testing.T, e *PostgresStreamBroker, age time.Duration, channels ...string) {
	_, err := e.pool.Exec(context.Background(), fmt.Sprintf(`
		UPDATE %s SET expires_at = expires_at - $1::interval, updated_at = updated_at - $1::interval
		 WHERE channel = ANY($2)`, e.names.meta), durationToIntervalString(age), channels)
	require.NoError(t, err)
}

func TestEmptyChannelMetaTTL(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		initial time.Duration
		metaTTL time.Duration
		want    time.Duration
	}{
		{"option off", 0, 100 * time.Hour, 100 * time.Hour},
		{"option on", 10 * time.Minute, 100 * time.Hour, 10 * time.Minute},
		{"option above meta TTL", 10 * time.Hour, time.Hour, time.Hour},
		// As before the option: reads keep the meta of an empty channel.
		{"option off: at least stream retention", 0, time.Second, 24 * time.Hour},
		// Below this a live subscriber could not keep the meta.
		{"option on: at least 3 position checks", 10 * time.Minute, time.Second, 2 * time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{}, centrifuge.Config{HistoryMetaTTLInitial: tt.initial})
			_, err := node.History("ttl", centrifuge.WithHistoryMetaTTL(tt.metaTTL))
			require.NoError(t, err)
			require.InDelta(t, tt.want.Seconds(), metaTTLLeft(t, e, "ttl").Seconds(), 30)
		})
	}
}

// A namespace's initial TTL comes with the read and works without the global
// option.
func TestEmptyChannelMetaTTLPerNamespace(t *testing.T) {
	t.Parallel()
	e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{})
	_, err := node.History("ns", centrifuge.WithHistoryMetaTTL(100*time.Hour), centrifuge.WithHistoryMetaTTLInitial(10*time.Minute))
	require.NoError(t, err)
	require.InDelta(t, (10 * time.Minute).Seconds(), metaTTLLeft(t, e, "ns").Seconds(), 30)
	require.NotNil(t, e.emptyMeta.Started(), "the keeper starts with the first initial TTL")
}

func TestEmptyChannelMetaTTLFirstPublication(t *testing.T) {
	t.Parallel()
	e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{}, centrifuge.Config{HistoryMetaTTLInitial: 10 * time.Minute})
	top, err := node.History("first", centrifuge.WithHistoryMetaTTL(100*time.Hour))
	require.NoError(t, err)
	require.InDelta(t, (10 * time.Minute).Seconds(), metaTTLLeft(t, e, "first").Seconds(), 30)

	res, err := node.Publish("first", []byte("x"), centrifuge.WithHistory(10, time.Minute, 100*time.Hour))
	require.NoError(t, err)
	require.Equal(t, top.Epoch, res.Epoch)
	require.InDelta(t, (100 * time.Hour).Seconds(), metaTTLLeft(t, e, "first").Seconds(), 30)

	// A read of a channel with publications does not shorten it.
	_, err = node.History("first", centrifuge.WithHistoryMetaTTL(100*time.Hour))
	require.NoError(t, err)
	require.InDelta(t, (100 * time.Hour).Seconds(), metaTTLLeft(t, e, "first").Seconds(), 30)
}

// Reads never move the expiry back, which pgemptymeta.Keeper relies on: a node
// remembers when a meta is due, and another may have a shorter TTL configured.
func TestEmptyChannelMetaNotShortened(t *testing.T) {
	t.Parallel()
	dsn := getPostgresConnString(t)
	before, node := newEpochTestNode(t, PostgresStreamBrokerConfig{DSN: dsn})
	_, err := node.History("monotone", centrifuge.WithHistoryMetaTTL(100*time.Hour))
	require.NoError(t, err)

	_, node = newEpochTestNode(t, PostgresStreamBrokerConfig{DSN: dsn}, centrifuge.Config{HistoryMetaTTLInitial: 10 * time.Minute})
	_, err = node.History("monotone", centrifuge.WithHistoryMetaTTL(100*time.Hour))
	require.NoError(t, err)
	require.InDelta(t, (100 * time.Hour).Seconds(), metaTTLLeft(t, before, "monotone").Seconds(), 30)
}

// Without an initial TTL the keeper never starts, as before the option: reads
// alone extend the meta of an empty channel.
func TestEmptyChannelMetaNoKeeperWithoutOption(t *testing.T) {
	t.Parallel()
	e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{})
	_, err := node.History("ch")
	require.NoError(t, err)
	require.Nil(t, e.emptyMeta.Started())
}

// Without fan-out centrifuge does no position checks with this broker, so the
// broker itself keeps the meta of empty channels which have subscribers.
func TestEmptyChannelMetaKeptWhileSubscribed(t *testing.T) {
	t.Parallel()
	e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{}, centrifuge.Config{
		HistoryMetaTTLInitial:           time.Hour,
		ClientChannelPositionCheckDelay: time.Second,
	})
	const ch = "idle"
	_, err := node.History(ch)
	require.NoError(t, err)
	ageMeta(t, e, 40*time.Minute, ch)
	e.emptyMeta.Get().HasSubscribers = func(string) bool { return true }
	e.emptyMeta.Get().Schedule(ch, 0, time.Now())

	require.Eventually(t, func() bool {
		return metaTTLLeft(t, e, ch) > 55*time.Minute
	}, 30*time.Second, 50*time.Millisecond)
}

func TestEmptyMetaKeeper(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const ch = "keeper"
	// The broker's own worker doesn't get to run: its interval is 20s and the
	// channels aren't due.
	newKeeperTest := func(t *testing.T, subscribed bool) (*PostgresStreamBroker, *centrifuge.Node) {
		e, node := newEpochTestNode(t, PostgresStreamBrokerConfig{}, centrifuge.Config{HistoryMetaTTLInitial: time.Hour})
		e.emptyMeta.Get().HasSubscribers = func(string) bool { return subscribed }
		_, err := node.History(ch)
		require.NoError(t, err)
		return e, node
	}
	whenDue := func() time.Time { return time.Now().Add(31 * time.Minute) }

	t.Run("no subscribers", func(t *testing.T) {
		t.Parallel()
		e, _ := newKeeperTest(t, false)
		require.False(t, e.emptyMeta.Get().IsDue(ch, whenDue()))
	})

	t.Run("queried only when due", func(t *testing.T) {
		t.Parallel()
		e, _ := newKeeperTest(t, true)
		require.False(t, e.emptyMeta.Get().IsDue(ch, time.Now().Add(29*time.Minute)))
		require.True(t, e.emptyMeta.Get().IsDue(ch, whenDue()))
	})

	t.Run("unsubscribed", func(t *testing.T) {
		t.Parallel()
		e, _ := newKeeperTest(t, true)
		require.NoError(t, e.Unsubscribe(ch))
		require.False(t, e.emptyMeta.Get().IsDue(ch, whenDue()))
	})

	t.Run("extended when less than half is left", func(t *testing.T) {
		t.Parallel()
		e, _ := newKeeperTest(t, true)
		ageMeta(t, e, 40*time.Minute, ch)
		now := whenDue()
		require.NoError(t, e.emptyMeta.Get().Keep(ctx, now))
		require.InDelta(t, time.Hour.Seconds(), metaTTLLeft(t, e, ch).Seconds(), 30)
		require.False(t, e.emptyMeta.Get().IsDue(ch, now.Add(29*time.Minute)))
	})

	t.Run("left alone with more than half left", func(t *testing.T) {
		t.Parallel()
		e, _ := newKeeperTest(t, true)
		ageMeta(t, e, 20*time.Minute, ch)
		require.NoError(t, e.emptyMeta.Get().Keep(ctx, whenDue()))
		require.InDelta(t, (40 * time.Minute).Seconds(), metaTTLLeft(t, e, ch).Seconds(), 30)
	})

	t.Run("published channel is dropped", func(t *testing.T) {
		t.Parallel()
		e, node := newKeeperTest(t, true)
		_, err := node.Publish(ch, []byte("x"), emptyEpochPublish)
		require.NoError(t, err)
		require.NoError(t, e.emptyMeta.Get().Keep(ctx, whenDue()))
		require.False(t, e.emptyMeta.Get().IsDue(ch, time.Now().Add(1000*time.Hour)))
	})

	t.Run("locked row is skipped and stays due", func(t *testing.T) {
		t.Parallel()
		e, _ := newKeeperTest(t, true)
		ageMeta(t, e, 40*time.Minute, ch)
		tx, err := e.pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE channel = $1 FOR UPDATE", e.names.meta), ch)
		require.NoError(t, err)
		waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		now := whenDue()
		require.NoError(t, e.emptyMeta.Get().Keep(waitCtx, now))
		require.NoError(t, waitCtx.Err(), "waited for a locked row")
		require.True(t, e.emptyMeta.Get().IsDue(ch, now))
	})

	t.Run("several batches next to a meta which never expires", func(t *testing.T) {
		t.Parallel()
		e, _ := newKeeperTest(t, true)
		const n = 2*pgemptymeta.BatchSize + 1
		_, err := e.pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s (channel, epoch, expires_at, updated_at)
			SELECT 'batch:' || i, 'e', NOW() + interval '20 minutes', NOW() - interval '40 minutes' FROM generate_series(1, $1::int) i`, e.names.meta), n)
		require.NoError(t, err)
		_, err = e.pool.Exec(ctx, fmt.Sprintf(
			"INSERT INTO %s (channel, epoch, top_offset, expires_at) VALUES ('forever', 'e', 1, 'infinity')", e.names.meta))
		require.NoError(t, err)
		now := time.Now()
		for i := range n {
			e.emptyMeta.Get().Schedule(fmt.Sprintf("batch:%d", i+1), 0, now)
		}
		e.emptyMeta.Get().Schedule("forever", 0, now)

		require.NoError(t, e.emptyMeta.Get().Keep(ctx, now))
		var kept int
		require.NoError(t, e.pool.QueryRow(ctx, fmt.Sprintf(
			"SELECT count(*) FROM %s WHERE channel LIKE 'batch:%%' AND expires_at > NOW() + interval '55 minutes'", e.names.meta)).Scan(&kept))
		require.Equal(t, n, kept)
		require.False(t, e.emptyMeta.Get().IsDue("batch:1", now))
		require.False(t, e.emptyMeta.Get().IsDue("forever", now))

		_, sp, err := e.History("forever", centrifuge.HistoryOptions{})
		require.NoError(t, err)
		require.Equal(t, uint64(1), sp.Offset)
	})
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
