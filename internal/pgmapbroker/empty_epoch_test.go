//go:build integration

package pgmapbroker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/pgtest"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

var (
	recoverableMap = centrifuge.MapChannelOptions{Mode: centrifuge.MapModeRecoverable, KeyTTL: time.Hour, MetaTTL: 100 * time.Hour}
	persistentMap  = centrifuge.MapChannelOptions{Mode: centrifuge.MapModePersistent}
	ephemeralMap   = centrifuge.MapChannelOptions{Mode: centrifuge.MapModeEphemeral, KeyTTL: time.Hour}
)

func newMapEpochBroker(t *testing.T, opts centrifuge.MapChannelOptions, conf centrifuge.Config) *PostgresMapBroker {
	conf.Map.GetMapChannelOptions = func(string) centrifuge.MapChannelOptions { return opts }
	node, err := centrifuge.New(conf)
	require.NoError(t, err)
	return newTestPostgresMapBroker(t, node)
}

func mapPublish(t *testing.T, e *PostgresMapBroker, ch, key string) centrifuge.StreamPosition {
	res, err := e.Publish(context.Background(), ch, key, centrifuge.MapPublishOptions{Data: []byte("x")})
	require.NoError(t, err)
	return res.Position
}

func mapStatePosition(t *testing.T, e *PostgresMapBroker, ch string) centrifuge.StreamPosition {
	res, err := e.ReadState(context.Background(), ch, centrifuge.MapReadStateOptions{Limit: 100})
	require.NoError(t, err)
	return res.Position
}

func forgetMapMeta(t *testing.T, e *PostgresMapBroker, ch string) {
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, fmt.Sprintf("UPDATE %s SET expires_at = NOW() - interval '1 second' WHERE channel = $1", e.names.meta), ch)
	require.NoError(t, err)
	e.cleanupEntries(ctx)
}

func mapMetaTTLLeft(t *testing.T, e *PostgresMapBroker, ch string) *float64 {
	var left *float64
	require.NoError(t, e.pool.QueryRow(context.Background(), fmt.Sprintf(
		"SELECT EXTRACT(EPOCH FROM expires_at - NOW())::float8 FROM %s WHERE channel = $1", e.names.meta), ch).Scan(&left))
	return left
}

func mapMetaRows(t *testing.T, e *PostgresMapBroker, ch string) int {
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(), fmt.Sprintf(
		"SELECT count(*) FROM %s WHERE channel = $1", e.names.meta), ch).Scan(&n))
	return n
}

// A position taken in an empty map must not match a later stream, nor the
// channel being empty again after one was created and forgotten.
func TestMapEmptyPositionDetectsForgottenStream(t *testing.T) {
	t.Parallel()
	for _, publishAfter := range []bool{true, false} {
		t.Run(fmt.Sprintf("publish after %v", publishAfter), func(t *testing.T) {
			t.Parallel()
			e := newMapEpochBroker(t, recoverableMap, centrifuge.Config{})
			const ch = "forgotten"
			since := mapStatePosition(t, e, ch)

			mapPublish(t, e, ch, "lost")
			forgetMapMeta(t, e, ch)
			if publishAfter {
				mapPublish(t, e, ch, "new")
			}

			_, err := e.ReadStream(context.Background(), ch, centrifuge.MapReadStreamOptions{
				Filter: centrifuge.StreamFilter{Since: &since, Limit: 100},
			})
			require.ErrorIs(t, err, centrifuge.ErrorUnrecoverablePosition)
		})
	}
}

// Without any loss a position taken in an empty map recovers everything.
func TestMapEmptyPositionRecovers(t *testing.T) {
	t.Parallel()
	e := newMapEpochBroker(t, recoverableMap, centrifuge.Config{})
	const ch = "recovers"
	since := mapStatePosition(t, e, ch)
	require.NotEmpty(t, since.Epoch)
	for i := range 3 {
		require.Equal(t, since.Epoch, mapPublish(t, e, ch, fmt.Sprint(i)).Epoch)
	}
	res, err := e.ReadStream(context.Background(), ch, centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{Since: &since, Limit: 100},
	})
	require.NoError(t, err)
	require.Len(t, res.Publications, 3)
}

// Every read which gives a subscriber a position gives an epoch.
func TestMapEmptyEpochReadPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stream := func(f centrifuge.StreamFilter) func(e *PostgresMapBroker, ch string) (centrifuge.StreamPosition, error) {
		return func(e *PostgresMapBroker, ch string) (centrifuge.StreamPosition, error) {
			res, err := e.ReadStream(ctx, ch, centrifuge.MapReadStreamOptions{Filter: f})
			return res.Position, err
		}
	}
	state := func(limit int) func(e *PostgresMapBroker, ch string) (centrifuge.StreamPosition, error) {
		return func(e *PostgresMapBroker, ch string) (centrifuge.StreamPosition, error) {
			res, err := e.ReadState(ctx, ch, centrifuge.MapReadStateOptions{Limit: limit})
			return res.Position, err
		}
	}
	for name, read := range map[string]func(e *PostgresMapBroker, ch string) (centrifuge.StreamPosition, error){
		"state page":      state(10),
		"state position":  state(0),
		"stream":          stream(centrifuge.StreamFilter{Limit: 10}),
		"stream position": stream(centrifuge.StreamFilter{}),
		"stream reverse":  stream(centrifuge.StreamFilter{Limit: 10, Reverse: true}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newMapEpochBroker(t, recoverableMap, centrifuge.Config{})
			pos, err := read(e, "empty")
			require.NoError(t, err)
			require.NotEmpty(t, pos.Epoch)
			again, err := read(e, "empty")
			require.NoError(t, err)
			require.Equal(t, pos, again)
		})
	}
}

// What stays blank, without a write: maps without a stream and reads of one
// key.
func TestMapEmptyEpochNotCreated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("ephemeral", func(t *testing.T) {
		t.Parallel()
		e := newMapEpochBroker(t, ephemeralMap, centrifuge.Config{})
		require.Empty(t, mapStatePosition(t, e, "ch").Epoch)
		require.Zero(t, mapMetaRows(t, e, "ch"))
	})
	t.Run("one key", func(t *testing.T) {
		t.Parallel()
		e := newMapEpochBroker(t, recoverableMap, centrifuge.Config{})
		_, err := e.ReadState(ctx, "ch", centrifuge.MapReadStateOptions{Key: "k"})
		require.NoError(t, err)
		require.Zero(t, mapMetaRows(t, e, "ch"))
	})
}

func TestMapEmptyStalePosition(t *testing.T) {
	t.Parallel()
	e := newMapEpochBroker(t, recoverableMap, centrifuge.Config{})
	stale := centrifuge.StreamPosition{Offset: 1, Epoch: "gone"}
	_, err := e.ReadStream(context.Background(), "ch", centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{Since: &stale, Limit: 10},
	})
	require.ErrorIs(t, err, centrifuge.ErrorUnrecoverablePosition)
	_, err = e.ReadState(context.Background(), "ch", centrifuge.MapReadStateOptions{Limit: 10, Revision: &stale})
	require.ErrorIs(t, err, centrifuge.ErrorUnrecoverablePosition)
}

// Cleanup leaves state rows behind. A new epoch must not show them.
func TestMapEmptyEpochWipesForgottenState(t *testing.T) {
	t.Parallel()
	e := newMapEpochBroker(t, recoverableMap, centrifuge.Config{})
	ctx := context.Background()
	const ch = "zombie"
	dead := mapPublish(t, e, ch, "old")
	forgetMapMeta(t, e, ch)

	res, err := e.ReadState(ctx, ch, centrifuge.MapReadStateOptions{Limit: 100})
	require.NoError(t, err)
	require.NotEmpty(t, res.Position.Epoch)
	require.NotEqual(t, dead.Epoch, res.Position.Epoch)
	require.Empty(t, res.Publications)
	stats, err := e.Stats(ctx, ch)
	require.NoError(t, err)
	require.Zero(t, stats.NumKeys)

	mapPublish(t, e, ch, "new")
	res, err = e.ReadState(ctx, ch, centrifuge.MapReadStateOptions{Limit: 100})
	require.NoError(t, err)
	require.Len(t, res.Publications, 1)
	require.Equal(t, "new", res.Publications[0].Key)
}

func TestMapEmptyMetaTTL(t *testing.T) {
	t.Parallel()
	shortLived := centrifuge.MapChannelOptions{Mode: centrifuge.MapModeRecoverable, KeyTTL: time.Second, StreamTTL: time.Second, MetaTTL: time.Second}
	for _, tt := range []struct {
		name        string
		opts        centrifuge.MapChannelOptions
		unpublished time.Duration
		want        time.Duration // zero: never expires
	}{
		{"recoverable", recoverableMap, 0, 100 * time.Hour},
		{"recoverable, option", recoverableMap, 10 * time.Minute, 10 * time.Minute},
		{"option above meta TTL", recoverableMap, 200 * time.Hour, 100 * time.Hour},
		{"persistent", persistentMap, 0, 0},
		{"persistent, option", persistentMap, 10 * time.Minute, 10 * time.Minute},
		{"at least 3 position checks", shortLived, 0, 2 * time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newMapEpochBroker(t, tt.opts, centrifuge.Config{HistoryMetaTTLUnpublished: tt.unpublished})
			require.NotEmpty(t, mapStatePosition(t, e, "ttl").Epoch)
			left := mapMetaTTLLeft(t, e, "ttl")
			if tt.want == 0 {
				require.Nil(t, left)
				return
			}
			require.NotNil(t, left)
			require.InDelta(t, tt.want.Seconds(), *left, 30)
		})
	}
}

// The TTL of an empty map must not outlive its first publication: a persistent
// map would lose its meta, and with it its state.
func TestMapFirstPublicationReplacesEmptyMetaTTL(t *testing.T) {
	t.Parallel()
	option := centrifuge.Config{HistoryMetaTTLUnpublished: 10 * time.Minute}
	const ch = "first"

	t.Run("persistent", func(t *testing.T) {
		t.Parallel()
		e := newMapEpochBroker(t, persistentMap, option)
		since := mapStatePosition(t, e, ch)
		require.NotNil(t, mapMetaTTLLeft(t, e, ch))
		require.Equal(t, since.Epoch, mapPublish(t, e, ch, "k").Epoch)
		require.Nil(t, mapMetaTTLLeft(t, e, ch))
	})
	t.Run("recoverable", func(t *testing.T) {
		t.Parallel()
		e := newMapEpochBroker(t, recoverableMap, option)
		require.NotEmpty(t, mapStatePosition(t, e, ch).Epoch)
		mapPublish(t, e, ch, "k")
		left := mapMetaTTLLeft(t, e, ch)
		require.NotNil(t, left)
		require.InDelta(t, (100 * time.Hour).Seconds(), *left, 30)
	})
	t.Run("SQL without p_meta_ttl", func(t *testing.T) {
		t.Parallel()
		// As documented for applications. The result must be the same as
		// without the read: permanent.
		e := newMapEpochBroker(t, recoverableMap, option)
		require.NotEmpty(t, mapStatePosition(t, e, ch).Epoch)
		_, err := e.pool.Exec(context.Background(), fmt.Sprintf(
			"SELECT * FROM %s(p_channel := $1, p_key := 'k', p_data := $2)", e.names.publish), ch, []byte("x"))
		require.NoError(t, err)
		require.Nil(t, mapMetaTTLLeft(t, e, ch))
	})
}

// A replica which doesn't have the meta row yet sends the read to the primary.
func TestMapEmptyEpochLaggingReplica(t *testing.T) {
	// The "replica" is another schema, so it never catches up.
	replica := newMapEpochBroker(t, recoverableMap, centrifuge.Config{})
	replicaDSN := replica.conf.DSN
	t.Run("primary", func(t *testing.T) {
		node, err := centrifuge.New(centrifuge.Config{Map: centrifuge.MapConfig{
			GetMapChannelOptions: func(string) centrifuge.MapChannelOptions { return recoverableMap },
		}})
		require.NoError(t, err)
		e, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
			DSN: pgtest.SchemaDSN(t), ReplicaDSN: []string{replicaDSN}, NumShards: 4, BinaryData: true,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = e.Close(context.Background()) })
		ctx := context.Background()
		require.NoError(t, e.EnsureSchema(ctx))
		cached := centrifuge.MapReadStateOptions{Limit: 100, AllowCached: true}

		pub := mapPublish(t, e, "published", "k")
		res, err := e.ReadState(ctx, "published", cached)
		require.NoError(t, err)
		require.Equal(t, pub, res.Position)
		require.Len(t, res.Publications, 1)

		first, err := e.ReadState(ctx, "empty", cached)
		require.NoError(t, err)
		require.NotEmpty(t, first.Position.Epoch)
		second, err := e.ReadState(ctx, "empty", cached)
		require.NoError(t, err)
		require.Equal(t, first.Position, second.Position)

		// The primary's position is good on a replica which doesn't have it
		// yet: live transition and the next state page both come with it.
		since := first.Position
		stream, err := e.ReadStream(ctx, "empty", centrifuge.MapReadStreamOptions{
			Filter: centrifuge.StreamFilter{Since: &since, Limit: 100}, AllowCached: true,
		})
		require.NoError(t, err)
		require.Equal(t, since, stream.Position)
		page, err := e.ReadState(ctx, "empty", centrifuge.MapReadStateOptions{Limit: 100, AllowCached: true, Revision: &since})
		require.NoError(t, err)
		require.Equal(t, since, page.Position)
	})
}

// Subscribing is enough to keep the meta: behind a cache the subscriber's
// reads don't reach this broker.
func TestMapEmptyMetaKeptWhileSubscribed(t *testing.T) {
	t.Parallel()
	e := newMapEpochBroker(t, persistentMap, centrifuge.Config{
		HistoryMetaTTLUnpublished:       time.Hour,
		ClientChannelPositionCheckDelay: time.Second,
	})
	const ch = "idle"
	require.NotEmpty(t, mapStatePosition(t, e, ch).Epoch)
	_, err := e.pool.Exec(context.Background(), fmt.Sprintf(`
		UPDATE %s SET expires_at = expires_at - interval '40 minutes', updated_at = updated_at - interval '40 minutes'
		 WHERE channel = $1`, e.names.meta), ch)
	require.NoError(t, err)
	require.NoError(t, e.Subscribe(ch))

	require.Eventually(t, func() bool {
		left := mapMetaTTLLeft(t, e, ch)
		return left != nil && *left > (55*time.Minute).Seconds()
	}, 30*time.Second, 50*time.Millisecond)
}

func TestMapEmptyMetaKeeperFollowsSubscription(t *testing.T) {
	t.Parallel()
	e := newMapEpochBroker(t, persistentMap, centrifuge.Config{HistoryMetaTTLUnpublished: time.Hour})
	const ch = "watched"
	later := time.Now().Add(2 * time.Hour)

	require.NotEmpty(t, mapStatePosition(t, e, ch).Epoch)
	require.False(t, e.emptyMetaKeeper.IsDue(ch, later), "read without a subscription")

	require.NoError(t, e.Subscribe(ch))
	mapStatePosition(t, e, ch)
	require.True(t, e.emptyMetaKeeper.IsDue(ch, later))

	require.NoError(t, e.Unsubscribe(ch))
	mapStatePosition(t, e, ch)
	require.False(t, e.emptyMetaKeeper.IsDue(ch, later), "read after unsubscribe")
}

// An existing database is at version 1: no ensure_meta, and a publish which
// keeps the TTL of the meta it finds.
func TestMapSchemaMigrationToV2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newMapEpochBroker(t, persistentMap, centrifuge.Config{HistoryMetaTTLUnpublished: 10 * time.Minute})
	for _, v := range []struct {
		prefix string
		binary bool
	}{{e.names.jsonbPrefix, false}, {e.names.binaryPrefix, true}} {
		_, funcs := splitSchemaSQL(renderSchema(v.prefix, v.binary))
		old := strings.Replace(funcs, "WHEN top_offset = 0 THEN NULL", "", 1)
		require.NotEqual(t, funcs, old)
		for _, q := range []string{
			old,
			fmt.Sprintf("DROP FUNCTION %sensure_meta", v.prefix),
			fmt.Sprintf("UPDATE %sschema_version SET schema_version = 1", v.prefix),
		} {
			_, err := e.pool.Exec(ctx, q)
			require.NoError(t, err)
		}
	}

	require.NoError(t, e.EnsureSchema(ctx))

	since := mapStatePosition(t, e, "ch")
	require.NotEmpty(t, since.Epoch)
	require.Equal(t, since.Epoch, mapPublish(t, e, "ch", "k").Epoch)
	require.Nil(t, mapMetaTTLLeft(t, e, "ch"))
	for _, prefix := range []string{e.names.jsonbPrefix, e.names.binaryPrefix} {
		var version int
		require.NoError(t, e.pool.QueryRow(ctx, fmt.Sprintf("SELECT schema_version FROM %sschema_version WHERE id = 1", prefix)).Scan(&version))
		require.Equal(t, 2, version)
	}
}
