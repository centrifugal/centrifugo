//go:build integration

package pgstreambroker

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

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
		FineGrainedHistoryCleanup: false,
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
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.history))
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
			fmt.Sprintf("DROP TABLE IF EXISTS %shistory CASCADE", prefix),
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
	var joinCount, leaveCount int32
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

	require.NoError(t, e.PublishJoin(channel, &centrifuge.ClientInfo{ClientID: "client-1", UserID: "user-1"}))
	require.NoError(t, e.PublishLeave(channel, &centrifuge.ClientInfo{ClientID: "client-1", UserID: "user-1"}))

	// Wait for outbox to deliver.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&joinCount) >= 1 && atomic.LoadInt32(&leaveCount) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
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

// TestPostgresStreamBroker_HistoryCreatesMetaOnFirstCall verifies that
// History() on a never-published channel creates a fresh meta row with a
// non-empty epoch.
func TestPostgresStreamBroker_HistoryCreatesMetaOnFirstCall(t *testing.T) {
	e, _, _ := newTestPostgresStreamBroker(t)

	channel := "test_history_creates_meta"
	pubs, sp, err := e.History(channel, centrifuge.HistoryOptions{
		Filter:  centrifuge.HistoryFilter{Limit: 10},
		MetaTTL: time.Hour,
	})
	require.NoError(t, err)
	require.Empty(t, pubs)
	require.Equal(t, uint64(0), sp.Offset)
	require.NotEmpty(t, sp.Epoch, "fresh meta should have a non-empty epoch")

	// A subsequent publish should reuse the same epoch.
	res, err := e.Publish(channel, []byte("x"), centrifuge.PublishOptions{
		HistoryTTL:  10 * time.Minute,
		HistorySize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, sp.Epoch, res.Epoch)
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

			// Verify meta still has a non-NULL meta_expires_at in the future.
			var metaAliveCount int
			err = e.pool.QueryRow(context.Background(), fmt.Sprintf(
				`SELECT COUNT(*) FROM %s WHERE channel = $1 AND meta_expires_at > NOW()`,
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
	require.Equal(t, customPrefix+"_binary_stream_history", e.names.history)

	t.Cleanup(func() {
		// Drop custom-prefixed tables.
		_, _ = e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", e.names.history))
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
