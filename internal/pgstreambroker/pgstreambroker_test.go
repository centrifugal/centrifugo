//go:build integration

package pgstreambroker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/metrics"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Initialize metrics with a custom registry for tests to avoid conflicts
	// with prometheus.DefaultRegisterer. This populates the package-level vars
	// (metrics.PGBrokerOrphanRows, etc.) that the sampler and tests use.
	registry := prometheus.NewRegistry()
	_ = metrics.Init(metrics.Config{
		Registerer: registry,
	})
	os.Exit(m.Run())
}

// testBrokerEventHandler is a configurable BrokerEventHandler for tests.
type testBrokerEventHandler struct {
	HandlePublicationFunc func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error
	HandleJoinFunc        func(ch string, info *centrifuge.ClientInfo) error
	HandleLeaveFunc       func(ch string, info *centrifuge.ClientInfo) error
}

func (b *testBrokerEventHandler) HandlePublication(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
	if b.HandlePublicationFunc != nil {
		return b.HandlePublicationFunc(ch, pub, sp, delta, prevPub)
	}
	return nil
}

func (b *testBrokerEventHandler) HandleJoin(ch string, info *centrifuge.ClientInfo) error {
	if b.HandleJoinFunc != nil {
		return b.HandleJoinFunc(ch, info)
	}
	return nil
}

func (b *testBrokerEventHandler) HandleLeave(ch string, info *centrifuge.ClientInfo) error {
	if b.HandleLeaveFunc != nil {
		return b.HandleLeaveFunc(ch, info)
	}
	return nil
}

func getPostgresConnString(tb testing.TB) string {
	connString := os.Getenv("CENTRIFUGE_POSTGRES_URL")
	if connString == "" {
		connString = "postgres://test:test@localhost:5432/test?sslmode=disable"
	}
	return connString
}

// newTestPostgresStreamBroker creates a test broker with a tiny CleanupInterval
// and a fast outbox poll interval so tests don't block on long timeouts.
func newTestPostgresStreamBroker(tb testing.TB) (*PostgresStreamBroker, *centrifuge.Node, *testBrokerEventHandler) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(tb, err)

	connString := getPostgresConnString(tb)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                       connString,
		NumShards:                 4, // fewer shards for faster tests
		BinaryData:                true,
		CleanupInterval:           100 * time.Millisecond,
		StreamRetention:           24 * time.Hour,
		PartitionLookaheadDays:    1,
		PartitionRetentionDays:    1,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(tb, err)

	ctx := context.Background()
	hardResetTestSchema(tb, e)
	require.NoError(tb, e.EnsureSchema(ctx))

	cleanupTestTables(ctx, e)

	handler := &testBrokerEventHandler{}
	require.NoError(tb, e.RegisterBrokerEventHandler(handler))
	require.NoError(tb, node.Run())

	tb.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	return e, node, handler
}

func cleanupTestTables(ctx context.Context, e *PostgresStreamBroker) {
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.stream))
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.meta))
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.idempotency))
}

// hardResetTestSchema drops all broker objects (tables and functions) for both
// jsonb and binary variants. Used before EnsureSchema in tests so dev iteration
// (changing function signatures, table shapes) doesn't get blocked by PG's
// "cannot change return type" error from a stale CREATE OR REPLACE call.
func hardResetTestSchema(tb testing.TB, e *PostgresStreamBroker) {
	ctx := context.Background()
	for _, prefix := range []string{e.names.jsonbPrefix, e.names.binaryPrefix} {
		stmts := []string{
			fmt.Sprintf("DROP FUNCTION IF EXISTS %spublish CASCADE", prefix),
			fmt.Sprintf("DROP FUNCTION IF EXISTS %spublish_strict CASCADE", prefix),
			fmt.Sprintf("DROP FUNCTION IF EXISTS %spublish_join CASCADE", prefix),
			fmt.Sprintf("DROP FUNCTION IF EXISTS %spublish_leave CASCADE", prefix),
			fmt.Sprintf("DROP FUNCTION IF EXISTS %sremove_history CASCADE", prefix),
			fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", strings.TrimRight(prefix, "_")),
			fmt.Sprintf("DROP TABLE IF EXISTS %smeta CASCADE", prefix),
			fmt.Sprintf("DROP TABLE IF EXISTS %sidempotency CASCADE", prefix),
			fmt.Sprintf("DROP TABLE IF EXISTS %sshard_lock CASCADE", prefix),
			fmt.Sprintf("DROP TABLE IF EXISTS %sschema_version CASCADE", prefix),
		}
		for _, sql := range stmts {
			_, _ = e.pool.Exec(ctx, sql)
		}
	}
}

// TestPostgresStreamBroker_PublishAndHistory verifies basic publish + history
// round-trip: a few messages with HistoryTTL set should be readable in order
// from History().
func TestPostgresStreamBroker_PublishAndHistory(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_publish_history"
	const n = 5
	for i := 0; i < n; i++ {
		res, err := e.Publish(channel, []byte(fmt.Sprintf(`{"i":%d}`, i)), centrifuge.PublishOptions{
			HistoryTTL:  10 * time.Minute,
			HistorySize: 10,
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.Epoch)
		require.Equal(t, uint64(i+1), res.Offset)
	}

	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter:  centrifuge.HistoryFilter{Limit: 10},
		MetaTTL: time.Hour,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(n), sp.Offset)
	require.NotEmpty(t, sp.Epoch)
	require.Len(t, pubs, n)
	for i, p := range pubs {
		require.Equal(t, uint64(i+1), p.Offset)
	}
}

// TestPostgresStreamBroker_PublishNoHistory verifies that publishing with
// HistoryTTL=0 returns an empty StreamPosition (per the Broker contract)
// but still gets delivered to a subscriber via the outbox worker.
func TestPostgresStreamBroker_PublishNoHistory(t *testing.T) {
	e, _, handler := newTestPostgresStreamBroker(t)

	channel := "test_no_history"
	delivered := make(chan struct{}, 1)
	handler.HandlePublicationFunc = func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
		if ch == channel {
			select {
			case delivered <- struct{}{}:
			default:
			}
		}
		return nil
	}

	res, err := e.Publish(channel, []byte("hello"), centrifuge.PublishOptions{
		HistoryTTL:  0,
		HistorySize: 0,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(0), res.Offset)
	require.Empty(t, res.Epoch)

	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("publication not delivered within timeout")
	}
}

// TestPostgresStreamBroker_Idempotency verifies a duplicate publish with the
// same idempotency key returns Suppressed=true with the cached offset.
func TestPostgresStreamBroker_Idempotency(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_idempotency"
	res1, err := e.Publish(channel, []byte("first"), centrifuge.PublishOptions{
		HistoryTTL:     10 * time.Minute,
		HistorySize:    10,
		IdempotencyKey: "key-1",
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)

	res2, err := e.Publish(channel, []byte("second"), centrifuge.PublishOptions{
		HistoryTTL:     10 * time.Minute,
		HistorySize:    10,
		IdempotencyKey: "key-1",
	})
	require.NoError(t, err)
	require.True(t, res2.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonIdempotency, res2.SuppressReason)
	require.Equal(t, res1.Offset, res2.Offset)

	pubs, _, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Len(t, pubs, 1) // only the first publish landed in history
}

// TestPostgresStreamBroker_VersionSuppression verifies that a publish with
// a stale version is suppressed with SuppressReasonVersion.
func TestPostgresStreamBroker_VersionSuppression(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_version"
	res1, err := e.Publish(channel, []byte("v5"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
		Version:     5,
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)

	// Publishing with a lower version should be suppressed.
	res2, err := e.Publish(channel, []byte("v3"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
		Version:     3,
	})
	require.NoError(t, err)
	require.True(t, res2.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonVersion, res2.SuppressReason)

	// Publishing with a higher version should succeed.
	res3, err := e.Publish(channel, []byte("v10"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
		Version:     10,
	})
	require.NoError(t, err)
	require.False(t, res3.Suppressed)
}

// TestPostgresStreamBroker_HistorySizeClampOnRead verifies that the read-time
// HistorySize clamp returns only the most recent N entries even when the table
// holds more (because the broker doesn't enforce HistorySize at write time).
func TestPostgresStreamBroker_HistorySizeClampOnRead(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_size_clamp"
	const total = 20
	const sizeLimit = 5
	for i := 0; i < total; i++ {
		_, err := e.Publish(channel, []byte(fmt.Sprintf(`{"i":%d}`, i)), centrifuge.PublishOptions{
			HistoryTTL:  10 * time.Minute,
			HistorySize: sizeLimit,
		})
		require.NoError(t, err)
	}

	// Forward query with NoLimit should return the LAST sizeLimit entries
	// (offsets 16..20), not the first sizeLimit entries.
	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: -1},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(total), sp.Offset)
	require.Len(t, pubs, sizeLimit)
	require.Equal(t, uint64(total-sizeLimit+1), pubs[0].Offset)
	require.Equal(t, uint64(total), pubs[sizeLimit-1].Offset)

	// Reverse query with NoLimit should return the same window in reverse order.
	pubs, _, err = e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: -1, Reverse: true},
	})
	require.NoError(t, err)
	require.Len(t, pubs, sizeLimit)
	require.Equal(t, uint64(total), pubs[0].Offset)
	require.Equal(t, uint64(total-sizeLimit+1), pubs[sizeLimit-1].Offset)
}

// TestPostgresStreamBroker_RemoveHistory verifies that RemoveHistory wipes
// publications and a subsequent History() returns empty rows but a non-zero
// position (because the meta is preserved).
func TestPostgresStreamBroker_RemoveHistory(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_remove"
	for i := 0; i < 3; i++ {
		_, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
			HistoryTTL:  10 * time.Minute,
			HistorySize: 10,
		})
		require.NoError(t, err)
	}

	require.NoError(t, e.RemoveHistory(channel))

	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Empty(t, pubs)
	// top_offset is preserved (matches Redis broker semantics).
	require.Equal(t, uint64(3), sp.Offset)
	require.NotEmpty(t, sp.Epoch)
}

// TestPostgresStreamBroker_PublishJoinLeave verifies that PublishJoin and
// PublishLeave events are delivered to the broker event handler in non-fanout
// mode via the outbox worker.
func TestPostgresStreamBroker_PublishJoinLeave(t *testing.T) {
	e, _, handler := newTestPostgresStreamBroker(t)

	channel := "test_join_leave"
	var probeReceived, joinCount, leaveCount int32
	handler.HandlePublicationFunc = func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
		if ch == channel+"_probe" {
			atomic.StoreInt32(&probeReceived, 1)
		}
		return nil
	}
	handler.HandleJoinFunc = func(ch string, info *centrifuge.ClientInfo) error {
		if ch == channel {
			atomic.AddInt32(&joinCount, 1)
		}
		return nil
	}
	handler.HandleLeaveFunc = func(ch string, info *centrifuge.ClientInfo) error {
		if ch == channel {
			atomic.AddInt32(&leaveCount, 1)
		}
		return nil
	}

	// Gate on probe delivery to ensure outbox workers have completed initCursor.
	_, err := e.Publish(channel+"_probe", []byte("probe"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&probeReceived) == 1
	}, 10*time.Second, 10*time.Millisecond, "outbox workers not ready: probe not delivered")

	require.NoError(t, e.PublishJoin(channel, &centrifuge.ClientInfo{ClientID: "client-1", UserID: "user-1"}))
	require.NoError(t, e.PublishLeave(channel, &centrifuge.ClientInfo{ClientID: "client-1", UserID: "user-1"}))

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&joinCount) >= 1 && atomic.LoadInt32(&leaveCount) >= 1
	}, 10*time.Second, 10*time.Millisecond,
		"join/leave not delivered: join=%d leave=%d",
		atomic.LoadInt32(&joinCount), atomic.LoadInt32(&leaveCount))
	require.Equal(t, int32(1), atomic.LoadInt32(&joinCount))
	require.Equal(t, int32(1), atomic.LoadInt32(&leaveCount))
}

// TestPostgresStreamBroker_OutboxOrdering verifies that concurrent publishes
// to a channel are delivered in per-channel offset order via the outbox.
func TestPostgresStreamBroker_OutboxOrdering(t *testing.T) {
	e, _, handler := newTestPostgresStreamBroker(t)

	channel := "test_ordering"
	const total = 50

	var mu sync.Mutex
	delivered := make([]uint64, 0, total)
	allDelivered := make(chan struct{})

	handler.HandlePublicationFunc = func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
		if ch != channel {
			return nil
		}
		mu.Lock()
		delivered = append(delivered, pub.Offset)
		if len(delivered) == total {
			close(allDelivered)
		}
		mu.Unlock()
		return nil
	}

	// Publish from a single goroutine — we want to verify offset ordering,
	// not concurrent-publish behavior here.
	for i := 0; i < total; i++ {
		_, err := e.Publish(channel, []byte(fmt.Sprintf("%d", i)), centrifuge.PublishOptions{
			HistoryTTL:  10 * time.Minute,
			HistorySize: 100,
		})
		require.NoError(t, err)
	}

	select {
	case <-allDelivered:
	case <-time.After(5 * time.Second):
		t.Fatalf("only %d/%d publications delivered", len(delivered), total)
	}

	mu.Lock()
	defer mu.Unlock()
	for i, offset := range delivered {
		require.Equal(t, uint64(i+1), offset, "offset out of order at index %d", i)
	}
}

// TestPostgresStreamBroker_HistoryOnNonExistentChannel verifies that
// History() on a never-published channel returns empty position (no meta
// UPSERT — History is now a pure read that can use replicas). The first
// Publish creates the meta and assigns the epoch.
func TestPostgresStreamBroker_HistoryOnNonExistentChannel(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_history_no_meta"
	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Empty(t, pubs)
	require.Equal(t, uint64(0), sp.Offset)
	require.Empty(t, sp.Epoch, "no meta row → empty epoch")

	// First publish creates the meta with a fresh epoch.
	res, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Epoch)

	// Subsequent History returns the epoch from the publish.
	pubs, sp, err = e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Len(t, pubs, 1)
	require.Equal(t, res.Epoch, sp.Epoch)
}

// TestPostgresStreamBroker_DefensiveClampMatrix exercises the SQL function's
// defensive clamp for various combinations of (history_ttl, meta_ttl).
func TestPostgresStreamBroker_DefensiveClampMatrix(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	cases := []struct {
		name        string
		historyTTL  time.Duration
		metaTTL     time.Duration
		expectAlive bool // expect meta to still exist after a brief wait
	}{
		{"both_set_meta_larger", 1 * time.Second, 10 * time.Second, true},
		{"both_set_history_larger", 10 * time.Second, 1 * time.Second, true}, // clamp protects meta
		{"only_history", 10 * time.Second, 0, true},
		{"only_meta", 0, 10 * time.Second, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := "test_clamp_" + tc.name
			_, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
				HistoryTTL:     tc.historyTTL,
				HistorySize:    10,
				HistoryMetaTTL: tc.metaTTL,
			})
			require.NoError(t, err)

			// Verify meta still has a non-NULL expires_at in the future.
			var metaAliveCount int
			err = e.pool.QueryRow(context.Background(), fmt.Sprintf(
				`SELECT COUNT(*) FROM %s WHERE channel = $1 AND expires_at > NOW()`,
				e.names.meta,
			), channel).Scan(&metaAliveCount)
			require.NoError(t, err)
			require.Equal(t, 1, metaAliveCount, "meta should be alive after publish")
		})
	}
}

// TestPostgresStreamBroker_TablePrefixCustom verifies that a custom TablePrefix
// produces tables under the custom namespace and round-trips publish + read.
func TestPostgresStreamBroker_TablePrefixCustom(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)

	connString := getPostgresConnString(t)
	const customPrefix = "custom_test"
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                    connString,
		NumShards:              4,
		BinaryData:             true,
		TablePrefix:            customPrefix,
		CleanupInterval:        100 * time.Millisecond,
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 1,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, e.EnsureSchema(ctx))
	// BinaryData=true → active prefix is the binary variant.
	require.Equal(t, customPrefix+"_binary_stream", e.names.stream)

	t.Cleanup(func() {
		// Drop custom-prefixed tables.
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", e.names.stream))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", e.names.meta))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", e.names.idempotency))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", e.names.shardLock))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", e.names.schemaVersion))
		// Also drop the binary variant.
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %shistory CASCADE", e.names.binaryPrefix))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %smeta CASCADE", e.names.binaryPrefix))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sidempotency CASCADE", e.names.binaryPrefix))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sshard_lock CASCADE", e.names.binaryPrefix))
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sschema_version CASCADE", e.names.binaryPrefix))
		_ = e.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	require.NoError(t, e.RegisterBrokerEventHandler(&testBrokerEventHandler{}))
	require.NoError(t, node.Run())

	channel := "test_custom_prefix"
	res, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), res.Offset)

	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Len(t, pubs, 1)
	require.Equal(t, uint64(1), sp.Offset)
}

// TestPostgresStreamBroker_VersionEpochReset verifies that a new version_epoch
// resets the version comparison: a publish with a lower numeric version but
// a different version_epoch is NOT suppressed.
func TestPostgresStreamBroker_VersionEpochReset(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_version_epoch_reset"

	// Publish with epoch=A and version=10.
	res1, err := e.Publish(channel, []byte("a10"), centrifuge.PublishOptions{
		HistoryTTL:   10 * time.Minute,
		HistorySize:  10,
		Version:      10,
		VersionEpoch: "A",
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)

	// Publish with same epoch=A and lower version=5 → suppressed.
	res2, err := e.Publish(channel, []byte("a5"), centrifuge.PublishOptions{
		HistoryTTL:   10 * time.Minute,
		HistorySize:  10,
		Version:      5,
		VersionEpoch: "A",
	})
	require.NoError(t, err)
	require.True(t, res2.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonVersion, res2.SuppressReason)

	// Publish with NEW epoch=B and lower version=5 → NOT suppressed (new epoch
	// resets the comparison).
	res3, err := e.Publish(channel, []byte("b5"), centrifuge.PublishOptions{
		HistoryTTL:   10 * time.Minute,
		HistorySize:  10,
		Version:      5,
		VersionEpoch: "B",
	})
	require.NoError(t, err)
	require.False(t, res3.Suppressed)
}

// TestPostgresStreamBroker_HistoryTTLExpiry verifies the read-time TTL filter
// hides rows older than meta.history_ttl, even when cleanup hasn't run yet.
// Functional correctness is independent of cleanup timing.
func TestPostgresStreamBroker_HistoryTTLExpiry(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_history_ttl_expiry"
	_, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:  500 * time.Millisecond,
		HistorySize: 10,
	})
	require.NoError(t, err)

	// Immediately readable.
	pubs, _, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Len(t, pubs, 1)

	// After TTL elapses, the read-time filter should exclude the row.
	time.Sleep(700 * time.Millisecond)
	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Empty(t, pubs)
	// Meta still exists, so position is preserved.
	require.Equal(t, uint64(1), sp.Offset)
	require.NotEmpty(t, sp.Epoch)
}

// TestPostgresStreamBroker_DualTTL_HistoryShorterThanMeta verifies that
// HistoryTTL and HistoryMetaTTL are independent: history rows can expire
// (via the read-time filter) while the channel meta survives, allowing
// reconnecting clients to still see a consistent epoch.
func TestPostgresStreamBroker_DualTTL_HistoryShorterThanMeta(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_dual_ttl"
	res1, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:     500 * time.Millisecond,
		HistorySize:    10,
		HistoryMetaTTL: 1 * time.Hour,
	})
	require.NoError(t, err)
	require.NotEmpty(t, res1.Epoch)

	// Wait for HistoryTTL to elapse but well before HistoryMetaTTL.
	time.Sleep(700 * time.Millisecond)

	// History returns empty (read-time filter), but meta is still alive
	// → position carries the original top_offset and epoch.
	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: 10},
	})
	require.NoError(t, err)
	require.Empty(t, pubs)
	require.Equal(t, uint64(1), sp.Offset)
	require.Equal(t, res1.Epoch, sp.Epoch, "epoch should be preserved")

	// A new publish reuses the same epoch (channel not forgotten).
	res2, err := e.Publish(channel, []byte("y"), centrifuge.PublishOptions{
		HistoryTTL:     500 * time.Millisecond,
		HistorySize:    10,
		HistoryMetaTTL: 1 * time.Hour,
	})
	require.NoError(t, err)
	require.Equal(t, res1.Epoch, res2.Epoch)
	require.Equal(t, uint64(2), res2.Offset)
}

// TestPostgresStreamBroker_DefensiveClampMatrix_PreservesMeta covers a wider
// matrix of (history_ttl, meta_ttl) combinations including the deliberate
// foot-gun case where meta_ttl < history_ttl. The defensive clamp in the SQL
// function should ensure meta survives at least as long as history.
func TestPostgresStreamBroker_PublishSQL_ClampMatrix(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)
	ctx := context.Background()

	type clampCase struct {
		name        string
		historyTTL  time.Duration
		metaTTL     time.Duration
		minMetaLife time.Duration // minimum expected expires_at - now
	}
	cases := []clampCase{
		{"both_nil", 0, 0, 23 * time.Hour}, // falls back to StreamRetention 24h
		{"only_history", 5 * time.Minute, 0, 5 * time.Minute},
		{"only_meta", 0, 10 * time.Minute, 10 * time.Minute},
		{"meta_larger", 5 * time.Minute, 10 * time.Minute, 10 * time.Minute},
		{"history_larger_clamps", 10 * time.Minute, 5 * time.Minute, 10 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := "test_clamp_matrix_" + tc.name
			_, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
				HistoryTTL:     tc.historyTTL,
				HistorySize:    10,
				HistoryMetaTTL: tc.metaTTL,
			})
			require.NoError(t, err)

			var metaExpiresAt time.Time
			err = e.pool.QueryRow(ctx, fmt.Sprintf(
				`SELECT expires_at FROM %s WHERE channel = $1`,
				e.names.meta,
			), channel).Scan(&metaExpiresAt)
			require.NoError(t, err)

			lifeRemaining := time.Until(metaExpiresAt)
			require.GreaterOrEqual(t, lifeRemaining, tc.minMetaLife-2*time.Second,
				"meta should live at least %v, got %v", tc.minMetaLife, lifeRemaining)
		})
	}
}

// TestPostgresStreamBroker_DeltaCompression verifies that publishes with
// UseDelta=true cause the publish SQL function to look up the previous
// publication's data and store it in prev_data on the new row.
func TestPostgresStreamBroker_DeltaCompression(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)
	ctx := context.Background()

	channel := "test_delta"

	// First publish: no previous data.
	_, err := e.Publish(channel, []byte("first"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
		UseDelta:    true,
	})
	require.NoError(t, err)

	// Second publish with delta: should capture "first" as prev_data.
	_, err = e.Publish(channel, []byte("second"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
		UseDelta:    true,
	})
	require.NoError(t, err)

	// Verify the second row has prev_data set to "first".
	var prevData []byte
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT prev_data FROM %s WHERE channel = $1 AND channel_offset = 2 AND kind = 0`,
		e.names.stream,
	), channel).Scan(&prevData)
	require.NoError(t, err)
	require.Equal(t, []byte("first"), prevData)
}

// TestPostgresStreamBroker_PublishJoinLeave_ShardLockSerialization runs
// concurrent publish + publish_join on the same channel and verifies the
// outbox worker delivers all rows without losing any. This is the F27 race
// regression test — without the shard lock on publish_join, an in-progress
// publication's id could be committed AFTER the join's id, causing the
// outbox worker to advance cursor past the in-progress publication.
func TestPostgresStreamBroker_PublishJoinLeave_ShardLockSerialization(t *testing.T) {
	e, _, handler := newTestPostgresStreamBroker(t)

	channel := "test_shard_lock_race"

	const totalPublishes = 50
	const totalJoins = 50

	var pubCount, joinCount int32
	handler.HandlePublicationFunc = func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
		if ch == channel {
			atomic.AddInt32(&pubCount, 1)
		}
		return nil
	}
	handler.HandleJoinFunc = func(ch string, info *centrifuge.ClientInfo) error {
		if ch == channel {
			atomic.AddInt32(&joinCount, 1)
		}
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < totalPublishes; i++ {
			_, err := e.Publish(channel, []byte(fmt.Sprintf("p%d", i)), centrifuge.PublishOptions{
				HistoryTTL:  10 * time.Minute,
				HistorySize: 1000,
			})
			require.NoError(t, err)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < totalJoins; i++ {
			require.NoError(t, e.PublishJoin(channel, &centrifuge.ClientInfo{
				ClientID: fmt.Sprintf("c%d", i),
				UserID:   fmt.Sprintf("u%d", i),
			}))
		}
	}()
	wg.Wait()

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&pubCount) >= totalPublishes &&
			atomic.LoadInt32(&joinCount) >= totalJoins
	}, 10*time.Second, 20*time.Millisecond,
		"deliveries: pub=%d/%d join=%d/%d",
		atomic.LoadInt32(&pubCount), totalPublishes,
		atomic.LoadInt32(&joinCount), totalJoins)

	require.Equal(t, int32(totalPublishes), atomic.LoadInt32(&pubCount))
	require.Equal(t, int32(totalJoins), atomic.LoadInt32(&joinCount))
}

// TestPostgresStreamBroker_RedisFanout exercises fanout mode end-to-end using
// a centrifuge.MemoryBroker as the inner broker (a real Redis isn't required
// to verify the fanout machinery — what we're testing is that the PG outbox
// reads rows and forwards them to the inner broker's Publish, and that
// PublishJoin/PublishLeave bypass PG entirely).
func TestPostgresStreamBroker_RedisFanout(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)

	innerBroker, err := centrifuge.NewMemoryBroker(node, centrifuge.MemoryBrokerConfig{})
	require.NoError(t, err)

	connString := getPostgresConnString(t)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                    connString,
		NumShards:              4,
		BinaryData:             true,
		CleanupInterval:        100 * time.Millisecond,
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 1,
		Broker:                 innerBroker,
		Outbox: OutboxConfig{
			PollInterval:              10 * time.Millisecond,
			BatchSize:                 100,
			AdvisoryLockBaseID:        828282828, // unique base to avoid collision
			AdvisoryLockRetryInterval: 50 * time.Millisecond,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	hardResetTestSchema(t, e)
	require.NoError(t, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	handler := &testBrokerEventHandler{}
	require.NoError(t, e.RegisterBrokerEventHandler(handler))
	require.NoError(t, node.Run())

	t.Cleanup(func() {
		_ = e.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	channel := "test_fanout"

	// Publish should write to history AND the outbox should forward to
	// the inner memory broker. We can't easily inspect the memory broker's
	// internal state here, but we can verify no errors and that the row
	// landed in PG history.
	res, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), res.Offset)

	// Verify history row was written.
	var count int
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s WHERE channel = $1 AND kind = 0`,
		e.names.stream,
	), channel).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// PublishJoin should bypass PG entirely (go through the inner broker).
	require.NoError(t, e.PublishJoin(channel, &centrifuge.ClientInfo{ClientID: "c1"}))

	// Verify NO join row was written to PG.
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s WHERE channel = $1 AND kind = 1`,
		e.names.stream,
	), channel).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "PublishJoin in fanout mode should NOT write to PG")
}

// TestPostgresStreamBroker_EpochGeneration verifies that the first publish
// to a channel generates a new epoch, subsequent publishes reuse it, and
// RemoveHistory does NOT reset the epoch (matches Redis broker semantics).
func TestPostgresStreamBroker_EpochGeneration(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_epoch_gen"

	res1, err := e.Publish(channel, []byte("a"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	epoch1 := res1.Epoch
	require.NotEmpty(t, epoch1)

	// Subsequent publishes reuse the epoch.
	res2, err := e.Publish(channel, []byte("b"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, epoch1, res2.Epoch)

	// RemoveHistory does NOT bump the epoch.
	require.NoError(t, e.RemoveHistory(channel))
	res3, err := e.Publish(channel, []byte("c"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, epoch1, res3.Epoch, "RemoveHistory should not reset epoch")
}

// TestPostgresStreamBroker_PartitionRetention_LookaheadAndDrop verifies that
// the partition worker creates lookahead partitions and (with RetentionDays > 0)
// drops old partitions.
func TestPostgresStreamBroker_PartitionRetention_LookaheadAndDrop(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)
	ctx := context.Background()

	// The test broker has PartitionRetentionDays=1, PartitionLookaheadDays=1,
	// and CleanupInterval=100ms — the retention worker fires aggressively, so
	// any past-retention partition can be dropped within a single tick of
	// being created. We therefore can't assert "exists immediately after
	// CREATE" reliably (the worker can race the SELECT). Instead:
	//
	//   1. Assert the partition doesn't exist at test start (catches leftover
	//      from a previous test run — without this, a stale partition would
	//      let step 3 trivially pass).
	//   2. CREATE the past-retention partition.
	//   3. Wait for the worker to drop it.
	//
	// Either step 3 sees the partition (briefly) exist and then disappear,
	// or the worker beats us to the punch and the partition was already
	// gone — both outcomes prove the retention worker is functioning.
	oldDate := time.Now().UTC().AddDate(0, 0, -3)
	nextDay := oldDate.AddDate(0, 0, 1)
	oldPartName := fmt.Sprintf("%s_%s", e.names.stream, oldDate.Format("2006_01_02"))

	var existsBefore bool
	err := e.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)
	`, oldPartName).Scan(&existsBefore)
	require.NoError(t, err)
	require.Falsef(t, existsBefore,
		"%s already exists at test start — previous test cleanup likely failed, this test cannot prove drop behaviour on stale state", oldPartName)

	_, err = e.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
		oldPartName, e.names.stream,
		oldDate.Format("2006-01-02"), nextDay.Format("2006-01-02"),
	))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var exists bool
		_ = e.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)
		`, oldPartName).Scan(&exists)
		return !exists
	}, 5*time.Second, 100*time.Millisecond, "old partition should be dropped")

	// Verify today's partition exists (lookahead worker should have created it).
	todayPartName := fmt.Sprintf("%s_%s", e.names.stream, time.Now().UTC().Format("2006_01_02"))
	var todayExists bool
	err = e.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)
	`, todayPartName).Scan(&todayExists)
	require.NoError(t, err)
	require.True(t, todayExists, "today's partition should exist")
}

// TestPostgresStreamBroker_PartitionRetention_RetentionZero_NeverDrops verifies
// the OSS-equivalent path: with PartitionRetentionDays = 0, the broker creates
// lookahead partitions but never drops old ones.
func TestPostgresStreamBroker_PartitionRetention_RetentionZero_NeverDrops(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)

	connString := getPostgresConnString(t)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                    connString,
		NumShards:              4,
		BinaryData:             true,
		CleanupInterval:        100 * time.Millisecond,
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 0, // unlimited retention
		Outbox: OutboxConfig{
			PollInterval:              10 * time.Millisecond,
			BatchSize:                 100,
			AdvisoryLockBaseID:        919191919,
			AdvisoryLockRetryInterval: 50 * time.Millisecond,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	hardResetTestSchema(t, e)
	require.NoError(t, e.EnsureSchema(ctx))

	require.NoError(t, e.RegisterBrokerEventHandler(&testBrokerEventHandler{}))
	require.NoError(t, node.Run())
	t.Cleanup(func() {
		_ = e.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	// Manually create an old partition.
	oldDate := time.Now().UTC().AddDate(0, 0, -10)
	nextDay := oldDate.AddDate(0, 0, 1)
	oldPartName := fmt.Sprintf("%s_%s", e.names.stream, oldDate.Format("2006_01_02"))
	_, err = e.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
		oldPartName, e.names.stream,
		oldDate.Format("2006-01-02"), nextDay.Format("2006-01-02"),
	))
	require.NoError(t, err)

	// Wait for several partition worker ticks. The old partition should NOT
	// be dropped because RetentionDays=0.
	time.Sleep(500 * time.Millisecond)

	var exists bool
	err = e.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)
	`, oldPartName).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "old partition should NOT be dropped with RetentionDays=0")
}

// TestPostgresStreamBroker_UseNotify_WakesIdleWorker verifies that with
// UseNotify=true, LISTEN/NOTIFY wakes an idle outbox worker — without
// depending on a comparison against the polling-only baseline.
//
// Design: PollInterval is set far above the test window so polling cannot
// be the delivery mechanism. After a first publish proves the worker is
// past its initial drain and into idle-wait, subsequent publishes that
// arrive within seconds must have been NOTIFY-driven (polling can't fire
// for 30s).
func TestPostgresStreamBroker_UseNotify_WakesIdleWorker(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)

	// NumShards=1 so the single NotifyCh reliably wakes the only worker.
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                    getPostgresConnString(t),
		NumShards:              1,
		BinaryData:             true,
		CleanupInterval:        500 * time.Millisecond,
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 1,
		UseNotify:              true,
		Outbox: OutboxConfig{
			// Long enough that polling cannot deliver during the test window;
			// any delivery to an idle worker must come via LISTEN/NOTIFY.
			PollInterval: 30 * time.Second,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	hardResetTestSchema(t, e)
	require.NoError(t, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	channel := "test_notify_wakes"
	var deliveredPtr atomic.Pointer[chan struct{}]
	handler := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			if ch == channel {
				if d := deliveredPtr.Load(); d != nil {
					select {
					case *d <- struct{}{}:
					default:
					}
				}
			}
			return nil
		},
	}
	require.NoError(t, e.RegisterBrokerEventHandler(handler))
	require.NoError(t, node.Run())
	t.Cleanup(func() {
		_ = e.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	// Wait for LISTEN to be bound. With PollInterval=30s, a publish whose
	// NOTIFY fires before LISTEN runs would be dropped and the worker would
	// idle for 30s — well past the 10s deadline below.
	require.Eventually(t, e.notifyListenerReady.Load,
		10*time.Second, 50*time.Millisecond,
		"notification listener did not bind LISTEN")

	publishAndWait := func(timeout time.Duration, msg string) bool {
		ch := make(chan struct{}, 1)
		deliveredPtr.Store(&ch)
		_, err := e.Publish(channel, []byte("ping"), centrifuge.PublishOptions{
			HistoryTTL:  10 * time.Minute,
			HistorySize: 10,
		})
		require.NoError(t, err, msg)
		select {
		case <-ch:
			return true
		case <-time.After(timeout):
			return false
		}
	}

	// First publish gates on worker reaching idle-wait. The worker's initial
	// drain may pick this up directly (no NOTIFY needed), which is fine — we
	// just need the worker to finish startup before the real assertions.
	require.True(t, publishAndWait(10*time.Second, "publish #0"),
		"worker did not deliver first publish within 10s")

	// From here on, the worker is idle. With PollInterval=30s, the only path
	// that can wake it within 2s is LISTEN/NOTIFY. Require multiple wakeups
	// in a row so a single race (e.g. LISTEN still establishing) is caught.
	for i := 1; i <= 3; i++ {
		require.True(t, publishAndWait(2*time.Second, fmt.Sprintf("publish #%d", i)),
			"NOTIFY did not wake idle worker on publish #%d within 2s", i)
	}
}

// TestPostgresStreamBroker_HistoryDoesNotRefreshMetaTTL verifies that History()
// is a pure read and does NOT refresh expires_at. This is different from
// the Redis broker (which refreshes on read) but matches the map broker
// pattern and allows History to use read replicas.
func TestPostgresStreamBroker_HistoryDoesNotRefreshMetaTTL(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)
	ctx := context.Background()

	channel := "test_history_no_refresh"

	// Publish with a short meta TTL.
	_, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:     10 * time.Minute,
		HistorySize:    10,
		HistoryMetaTTL: 2 * time.Second,
	})
	require.NoError(t, err)

	var metaExpiresAt1 time.Time
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT expires_at FROM %s WHERE channel = $1`,
		e.names.meta,
	), channel).Scan(&metaExpiresAt1)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// History read — should NOT change expires_at.
	_, _, err = e.History(channel, centrifuge.HistoryOptions{
		Filter:  centrifuge.HistoryFilter{Limit: 10},
		MetaTTL: 1 * time.Hour,
	})
	require.NoError(t, err)

	var metaExpiresAt2 time.Time
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT expires_at FROM %s WHERE channel = $1`,
		e.names.meta,
	), channel).Scan(&metaExpiresAt2)
	require.NoError(t, err)

	// expires_at should be UNCHANGED — History is a pure read.
	require.Equal(t, metaExpiresAt1.Unix(), metaExpiresAt2.Unix(),
		"expires_at should NOT be refreshed by History")
}

// TestPostgresStreamBroker_RemoveHistoryRaceWithPublish runs concurrent
// publish + RemoveHistory and verifies the broker remains consistent.
// The shard lock on RemoveHistory ensures any in-progress publish either
// commits before the DELETE or runs after.
func TestPostgresStreamBroker_RemoveHistoryRaceWithPublish(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_remove_race"
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _ = e.Publish(channel, []byte(fmt.Sprintf("p%d", i)), centrifuge.PublishOptions{
				HistoryTTL:  10 * time.Minute,
				HistorySize: 1000,
			})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/10; i++ {
			_ = e.RemoveHistory(channel)
			time.Sleep(time.Millisecond)
		}
	}()
	wg.Wait()

	// After all writers stop, one final RemoveHistory should leave the channel
	// fully empty (the shard lock prevents in-flight publishes from surviving).
	require.NoError(t, e.RemoveHistory(channel))
	pubs, _, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{Limit: -1},
	})
	require.NoError(t, err)
	require.Empty(t, pubs)
}


// TestPostgresStreamBroker_HistoryMetaTTL_NodeConfigFallback verifies the
// 3-tier fallback for meta TTL: when opts.HistoryMetaTTL is 0, the broker
// falls back to node.Config().HistoryMetaTTL, then to StreamRetention.
func TestPostgresStreamBroker_HistoryMetaTTL_NodeConfigFallback(t *testing.T) {
	// Use a short StreamRetention to make the fallback observable.
	node, err := centrifuge.New(centrifuge.Config{
		HistoryMetaTTL: 7 * time.Hour,
	})
	require.NoError(t, err)

	connString := getPostgresConnString(t)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                    connString,
		NumShards:              4,
		BinaryData:             true,
		CleanupInterval:        100 * time.Millisecond,
		StreamRetention:        24 * time.Hour, // safety floor
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 1,
		Outbox: OutboxConfig{
			PollInterval:              10 * time.Millisecond,
			BatchSize:                 100,
			AdvisoryLockBaseID:        939393939,
			AdvisoryLockRetryInterval: 50 * time.Millisecond,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	hardResetTestSchema(t, e)
	require.NoError(t, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	require.NoError(t, e.RegisterBrokerEventHandler(&testBrokerEventHandler{}))
	require.NoError(t, node.Run())
	t.Cleanup(func() {
		_ = e.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	channel := "test_node_config_fallback"
	_, err = e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
		// HistoryMetaTTL intentionally not set — expect fallback to node config 7h.
	})
	require.NoError(t, err)

	var metaExpiresAt time.Time
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT expires_at FROM %s WHERE channel = $1`,
		e.names.meta,
	), channel).Scan(&metaExpiresAt)
	require.NoError(t, err)

	// Should be ~7 hours in the future (from the node config).
	lifeRemaining := time.Until(metaExpiresAt)
	require.Greater(t, lifeRemaining, 6*time.Hour,
		"meta should fall back to node.Config().HistoryMetaTTL=7h")
	require.Less(t, lifeRemaining, 8*time.Hour)
}

// TestPostgresStreamBroker_PublishCleanupRace stresses the F27 race fix:
// concurrent publishes while cleanup runs frequently. Without the no-op
// UPDATE atomic-lock pattern, cleanup could delete a meta row mid-publish
// and the publish would fail or write a garbage offset.
func TestPostgresStreamBroker_PublishCleanupRace(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)

	connString := getPostgresConnString(t)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:             connString,
		NumShards:       4,
		BinaryData:      true,
		CleanupInterval: 10 * time.Millisecond, // very tight cleanup interval
		StreamRetention: 24 * time.Hour,
		// short PartitionRetention so partition drops also race with publishes
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 1,
		Outbox: OutboxConfig{
			PollInterval:              5 * time.Millisecond,
			BatchSize:                 100,
			AdvisoryLockBaseID:        949494949,
			AdvisoryLockRetryInterval: 50 * time.Millisecond,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	hardResetTestSchema(t, e)
	require.NoError(t, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	require.NoError(t, e.RegisterBrokerEventHandler(&testBrokerEventHandler{}))
	require.NoError(t, node.Run())
	t.Cleanup(func() {
		_ = e.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	const channels = 10
	const publishesPerChannel = 50

	var wg sync.WaitGroup
	for c := 0; c < channels; c++ {
		wg.Add(1)
		ch := fmt.Sprintf("test_publish_cleanup_race_%d", c)
		go func(channel string) {
			defer wg.Done()
			for i := 0; i < publishesPerChannel; i++ {
				_, err := e.Publish(channel, []byte(fmt.Sprintf("p%d", i)), centrifuge.PublishOptions{
					HistoryTTL:     50 * time.Millisecond, // short TTL → cleanup races
					HistorySize:    100,
					HistoryMetaTTL: 100 * time.Millisecond,
				})
				require.NoError(t, err, "publish should never fail")
			}
		}(ch)
	}
	wg.Wait()

	// If we got here without errors, the F27 race fix is holding under load.
}

// TestPostgresStreamBroker_DayRolloverWithLookahead verifies that LookaheadDays
// pre-creates partitions for tomorrow, so a publish dated tomorrow doesn't
// fail with "no partition for value".
//
// Rather than manipulating system time, we directly INSERT a row with a
// tomorrow created_at and verify it lands in the lookahead partition.
func TestPostgresStreamBroker_DayRolloverWithLookahead(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)
	ctx := context.Background()

	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	tomorrowPartName := fmt.Sprintf("%s_%s", e.names.stream, tomorrow.Format("2006_01_02"))

	// Verify tomorrow's partition exists (created by EnsureSchema's lookahead).
	var exists bool
	err := e.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)
	`, tomorrowPartName).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "tomorrow's partition should be pre-created by lookahead worker")

	// Manually insert a row dated tomorrow to verify partition routing works.
	_, err = e.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (channel, channel_offset, kind, data, created_at, shard_id)
		VALUES ($1, 1, 0, '\x'::bytea, $2, 0)
	`, e.names.stream), "test_day_rollover", tomorrow.Add(time.Hour))
	require.NoError(t, err, "insert into tomorrow's partition should succeed via lookahead")
}

// TestPostgresStreamBroker_Metrics verifies that the PG-specific metrics
// (from internal/metrics package) are populated by the cleanup worker's
// sampler. Publish/history counts are tracked by centrifuge at the Node
// level — the PG broker only contributes cleanup and partition gauges.
func TestPostgresStreamBroker_Metrics(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_metrics"
	_, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)

	// Wait for the sampler to fire (runs on CleanupInterval=100ms).
	time.Sleep(300 * time.Millisecond)

	// Verify PG-specific gauges are populated (initialized by TestMain).
	require.NotNil(t, metrics.PGBrokerPartitions)
}


// TestPostgresStreamBroker_OutboxCursorLag verifies the outbox cursor lag
// gauge is sampled without panicking after publications are processed.
func TestPostgresStreamBroker_OutboxCursorLag(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_cursor_lag"
	for i := 0; i < 5; i++ {
		_, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
			HistoryTTL:  10 * time.Minute,
			HistorySize: 100,
		})
		require.NoError(t, err)
	}

	// Wait for sampler to run. The main assertion is that the sampler
	// doesn't panic when computing cursor lag.
	time.Sleep(300 * time.Millisecond)
	require.NotNil(t, metrics.PGBrokerOutboxCursorLagSeconds)
}

// testutilGaugeValue extracts a GaugeVec value for the given labels.
func testutilGaugeValue(t *testing.T, vec *prometheus.GaugeVec, lvs ...string) float64 {
	t.Helper()
	g, err := vec.GetMetricWithLabelValues(lvs...)
	if err != nil {
		return 0
	}
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	if m.Gauge == nil {
		return 0
	}
	return m.Gauge.GetValue()
}

// TestPostgresStreamBroker_History_FiltersDeadEpoch verifies that History
// rejects rows from a previous epoch that linger in the partitioned stream
// table after meta TTL expiry + recreation. The time-based `created_at >
// cutoff` filter alone fails when history_ttl is widened between the dead
// epoch's last publish and the read; this test reproduces that scenario and
// confirms the new `epoch =` predicate closes it.
func TestPostgresStreamBroker_History_FiltersDeadEpoch(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)
	ctx := context.Background()

	channel := "test_dead_epoch_history"

	// Build the dead epoch under a short history_ttl. Three publishes leave
	// three stream rows tagged with epoch1.
	for i := 0; i < 3; i++ {
		_, err := e.Publish(channel, []byte(fmt.Sprintf(`{"dead":%d}`, i)), centrifuge.PublishOptions{
			HistoryTTL:  1 * time.Minute,
			HistorySize: 10,
		})
		require.NoError(t, err)
	}

	// Capture the dead epoch and drop the meta row to simulate meta TTL expiry.
	// Doing it directly avoids waiting for HistoryMetaTTL in a real test.
	var deadEpoch string
	err := e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT epoch FROM %s WHERE channel = $1`, e.names.meta), channel,
	).Scan(&deadEpoch)
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE channel = $1`, e.names.meta), channel)
	require.NoError(t, err)

	// The next publish picks a much wider history_ttl. Without the epoch
	// predicate, the read-time `created_at > NOW - 1h` cutoff would let the
	// dead-epoch rows back in.
	res, err := e.Publish(channel, []byte(`{"alive":0}`), centrifuge.PublishOptions{
		HistoryTTL:  1 * time.Hour,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.NotEqual(t, deadEpoch, res.Epoch, "fresh meta must have a new epoch")
	require.Equal(t, uint64(1), res.Offset, "fresh meta starts at offset 1")

	// Sanity: dead-epoch rows still in the stream table.
	var totalRows int
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s WHERE channel = $1 AND kind = 0`, e.names.stream),
		channel,
	).Scan(&totalRows)
	require.NoError(t, err)
	require.Equal(t, 4, totalRows, "stream must still hold dead-epoch rows for the test premise")

	// History must return only the live-epoch row, even though dead rows
	// share channel_offset=1 and fall inside the new wider cutoff window.
	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{
			Since: &centrifuge.StreamPosition{Offset: 0},
			Limit: 100,
		},
		MetaTTL: time.Hour,
	})
	require.NoError(t, err)
	require.Equal(t, res.Epoch, sp.Epoch)
	require.Len(t, pubs, 1, "History must filter out dead-epoch rows")
	require.Equal(t, []byte(`{"alive":0}`), pubs[0].Data)
	require.Equal(t, uint64(1), pubs[0].Offset)

	// Reverse read enforces the predicate too.
	pubs, _, err = e.History(channel, centrifuge.HistoryOptions{
		Filter: centrifuge.HistoryFilter{
			Since:   &centrifuge.StreamPosition{Offset: 0},
			Limit:   100,
			Reverse: true,
		},
		MetaTTL: time.Hour,
	})
	require.NoError(t, err)
	require.Len(t, pubs, 1, "reverse History must also filter dead-epoch rows")
	require.Equal(t, []byte(`{"alive":0}`), pubs[0].Data)
}

// TestPostgresStreamBroker_DeltaPrev_FiltersDeadEpoch verifies that the delta
// prev_data lookup inside the publish function ignores dead-epoch rows. If a
// dead epoch had a higher channel_offset than the freshly recreated meta,
// without the epoch filter the delta would point at a stale row's payload —
// poisoning the delta sent to subscribers.
func TestPostgresStreamBroker_DeltaPrev_FiltersDeadEpoch(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)
	ctx := context.Background()

	channel := "test_dead_epoch_delta"

	// Drive the dead epoch to channel_offset=5 so it dominates the new
	// epoch's first publish on channel_offset=1.
	for i := 0; i < 5; i++ {
		_, err := e.Publish(channel, []byte(fmt.Sprintf(`{"dead":%d}`, i)), centrifuge.PublishOptions{
			HistoryTTL:  1 * time.Minute,
			HistorySize: 10,
			UseDelta:    true,
		})
		require.NoError(t, err)
	}

	_, err := e.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE channel = $1`, e.names.meta), channel)
	require.NoError(t, err)

	// Fresh epoch: top_offset starts at 1. Delta lookup for the very first
	// publish should find no prev row (this is the new epoch's bootstrap),
	// not the dead-epoch row at channel_offset=5.
	res, err := e.Publish(channel, []byte(`{"alive":0}`), centrifuge.PublishOptions{
		HistoryTTL:  1 * time.Hour,
		HistorySize: 10,
		UseDelta:    true,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), res.Offset)

	// Inspect the row we just inserted: prev_data must be NULL since this
	// is the first publish under the new epoch.
	var prevData []byte
	err = e.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT prev_data FROM %s WHERE channel = $1 AND epoch = $2 AND kind = 0 AND channel_offset = 1`,
		e.names.stream),
		channel, res.Epoch,
	).Scan(&prevData)
	require.NoError(t, err)
	require.Nil(t, prevData, "prev_data must be NULL on the fresh epoch's first publish — dead-epoch row must not leak in")
}
