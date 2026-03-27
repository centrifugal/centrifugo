//go:build integration

package pgmapbroker

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// testBrokerEventHandler is a local copy of centrifuge's unexported test helper.
type testBrokerEventHandler struct {
	HandlePublicationFunc func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error
	HandleJoinFunc        func(ch string, info *centrifuge.ClientInfo) error
	HandleLeaveFunc       func(ch string, info *centrifuge.ClientInfo) error
	HandleControlFunc     func([]byte) error
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

func (b *testBrokerEventHandler) HandleControl(data []byte) error {
	if b.HandleControlFunc != nil {
		return b.HandleControlFunc(data)
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

// newTestPostgresMapBroker creates a test broker with default outbox mode.
func newTestPostgresMapBroker(tb testing.TB, n *centrifuge.Node) *PostgresMapBroker {
	return newTestPostgresMapBrokerWithOutbox(tb, n)
}

// newTestPostgresMapBrokerWithOutbox creates a test broker with outbox mode (default).
func newTestPostgresMapBrokerWithOutbox(tb testing.TB, n *centrifuge.Node) *PostgresMapBroker {
	connString := getPostgresConnString(tb)

	e, err := NewPostgresMapBroker(n, PostgresMapBrokerConfig{
		DSN:        connString,
		NumShards:  4, // Fewer shards for faster tests
		BinaryData: true,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(tb, err)

	ctx := context.Background()
	require.NoError(tb, e.EnsureSchema(ctx))

	// Clean up tables before test
	cleanupTestTables(ctx, e)

	err = e.RegisterEventHandler(nil)
	require.NoError(tb, err)

	tb.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = n.Shutdown(context.Background())
	})
	return e
}

func cleanupTestTables(ctx context.Context, e *PostgresMapBroker) {
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.stream))
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.state))
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.meta))
	_, _ = e.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE channel LIKE 'test_%%'", e.names.idempotency))
}

// stateToMapPostgres converts []Publication to map for easier testing.
func stateToMapPostgres(pubs []*centrifuge.Publication) map[string][]byte {
	result := make(map[string][]byte, len(pubs))
	for _, pub := range pubs {
		result[pub.Key] = pub.Data
	}
	return result
}

// TestPostgresMapBroker_StatefulChannel tests stateful channel with keyed state and revisions.
func TestPostgresMapBroker_StatefulChannel(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_stateful"

	// Publish some keyed state updates
	_, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte("data1"),
	})
	require.NoError(t, err)

	_, err = broker.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data: []byte("data2"),
	})
	require.NoError(t, err)

	_, err = broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte("data1_updated"),
	})
	require.NoError(t, err)

	// Read state
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	entries, streamPos, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.NotEmpty(t, streamPos.Epoch)
	require.Greater(t, streamPos.Offset, uint64(0))

	// Verify state contains latest values
	state := stateToMapPostgres(entries)
	require.Len(t, state, 2)
	require.Equal(t, []byte("data1_updated"), state["key1"])
	require.Equal(t, []byte("data2"), state["key2"])

	// Read stream to verify all publications are in history
	streamResult, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{
			Limit: -1, // Get all
		},
	})
	require.NoError(t, err)
	require.Len(t, streamResult.Publications, 3) // All 3 publications in stream
}

// TestPostgresMapBroker_StatefulChannelOrdered tests ordered stateful channel.
func TestPostgresMapBroker_StatefulChannelOrdered(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Ordered:       true,
				StreamSize:    100,
				StreamTTL:     300 * time.Second,
				KeyTTL:        300 * time.Second,
				Mode: centrifuge.MapModeDurable,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_ordered"

	// Publish with scores for ordering
	for i := 0; i < 5; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data:  []byte(fmt.Sprintf("data%d", i)),
			Score: int64(i * 10), // Scores: 0, 10, 20, 30, 40
		})
		require.NoError(t, err)
	}

	// Read ordered state (descending by score)
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 5)

	// Verify all keys present
	state := stateToMapPostgres(entries)
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key%d", i)
		require.Contains(t, state, key)
	}
}

// TestPostgresMapBroker_StateRevision tests that state values include revisions.
func TestPostgresMapBroker_StateRevision(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_revision"

	// Publish a keyed state update
	res1, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte("data1"),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), res1.Position.Offset)

	// Publish another update
	res2, err := broker.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data: []byte("data2"),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), res2.Position.Offset)
	require.Equal(t, res1.Position.Epoch, res2.Position.Epoch) // Same epoch

	// Read state - entries now include per-entry revisions
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	entries, streamPos, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Equal(t, res2.Position.Offset, streamPos.Offset)
	require.Equal(t, res2.Position.Epoch, streamPos.Epoch)

	// Verify payloads
	state := stateToMapPostgres(entries)
	require.Equal(t, []byte("data1"), state["key1"])
	require.Equal(t, []byte("data2"), state["key2"])

	// Verify per-entry offsets
	require.NotEmpty(t, streamPos.Epoch)
	for _, pub := range entries {
		require.Greater(t, pub.Offset, uint64(0))
	}
}

// TestPostgresMapBroker_StatePagination tests cursor-based state pagination.
func TestPostgresMapBroker_StatePagination(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_pagination"

	// Publish 10 keyed entries
	for i := 0; i < 10; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte(fmt.Sprintf("data%d", i)),
		})
		require.NoError(t, err)
	}

	// Read state with limit
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit:  3,
		Cursor: "",
	})
	page1, pos1, cursor := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.NotEmpty(t, page1)

	// Collect all keys across pages
	allKeys := make(map[string]bool)
	for _, entry := range page1 {
		allKeys[entry.Key] = true
	}

	// Continue reading until cursor is empty
	for cursor != "" {
		stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
			Limit:  3,
			Cursor: cursor,
		})
		page, pos, newCursor := stateRes.Publications, stateRes.Position, stateRes.Cursor
		require.NoError(t, err)
		require.Equal(t, pos1.Epoch, pos.Epoch) // Same epoch across pages

		for _, entry := range page {
			// Keys should not repeat across pages
			require.NotContains(t, allKeys, entry.Key, "key should not repeat: %s", entry.Key)
			allKeys[entry.Key] = true
		}
		cursor = newCursor
	}

	// Should have read all 10 entries
	require.Len(t, allKeys, 10)
}

// TestPostgresMapBroker_StreamRecovery tests stream recovery.
func TestPostgresMapBroker_StreamRecovery(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_recovery"

	// Publish some updates
	for i := 1; i <= 5; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte(fmt.Sprintf("data%d", i)),
		})
		require.NoError(t, err)
	}

	// Get position after first 2 messages
	sincePos := centrifuge.StreamPosition{Offset: 2}

	// Read stream since position 2
	streamResult, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{
			Since: &sincePos,
			Limit: -1,
		},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(5), streamResult.Position.Offset)
	require.Len(t, streamResult.Publications, 3) // Should get messages 3, 4, 5
}

// TestPostgresMapBroker_Idempotency tests idempotent publishing.
func TestPostgresMapBroker_Idempotency(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_idempotency"

	// Publish with idempotency key
	res1, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:                []byte("data1"),
		IdempotencyKey:      "unique-id-1",
		IdempotentResultTTL: 60 * time.Second,
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)
	require.Equal(t, uint64(1), res1.Position.Offset)

	// Publish again with same idempotency key
	res2, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:                []byte("data1_different"),
		IdempotencyKey:      "unique-id-1",
		IdempotentResultTTL: 60 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, res2.Suppressed)                                 // Suppressed due to idempotency
	require.Equal(t, centrifuge.SuppressReasonIdempotency, res2.SuppressReason) // The idempotency check returns the original offset
	require.Equal(t, res1.Position.Offset, res2.Position.Offset)     // Same offset

	// State should still have original data
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	state := stateToMapPostgres(entries)
	require.Len(t, state, 1)
	require.Equal(t, []byte("data1"), state["key1"])
}

// TestPostgresMapBroker_KeyMode tests KeyMode (IfNew, IfExists).
func TestPostgresMapBroker_KeyMode(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_keymode"

	// First publish with centrifuge.KeyModeIfNew should succeed
	res1, err := broker.Publish(ctx, channel, "slot1", centrifuge.MapPublishOptions{
		Data:    []byte("player1"),
		KeyMode: centrifuge.KeyModeIfNew,
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)
	require.Equal(t, uint64(1), res1.Position.Offset)

	// Second publish with centrifuge.KeyModeIfNew should be suppressed
	res2, err := broker.Publish(ctx, channel, "slot1", centrifuge.MapPublishOptions{
		Data:    []byte("player2"),
		KeyMode: centrifuge.KeyModeIfNew,
	})
	require.NoError(t, err)
	require.True(t, res2.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonKeyExists, res2.SuppressReason)

	// Verify state still has original data
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, []byte("player1"), entries[0].Data)

	// centrifuge.KeyModeIfExists should be suppressed for non-existent key
	res3, err := broker.Publish(ctx, channel, "nonexistent", centrifuge.MapPublishOptions{
		Data:    []byte("data"),
		KeyMode: centrifuge.KeyModeIfExists,
	})
	require.NoError(t, err)
	require.True(t, res3.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonKeyNotFound, res3.SuppressReason)

	// centrifuge.KeyModeIfExists should succeed for existing key
	res4, err := broker.Publish(ctx, channel, "slot1", centrifuge.MapPublishOptions{
		Data:    []byte("updated"),
		KeyMode: centrifuge.KeyModeIfExists,
	})
	require.NoError(t, err)
	require.False(t, res4.Suppressed)
}

// TestPostgresMapBroker_CAS tests Compare-And-Swap operations.
func TestPostgresMapBroker_CAS(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_cas"

	// Initial publish
	res1, err := broker.Publish(ctx, channel, "item1", centrifuge.MapPublishOptions{
		Data: []byte(`{"stock": 10}`),
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)

	// Read current state
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Key: "item1",
	})
	entries, pos, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// CAS update with correct position
	expectedPos := centrifuge.StreamPosition{Offset: entries[0].Offset, Epoch: pos.Epoch}
	res2, err := broker.Publish(ctx, channel, "item1", centrifuge.MapPublishOptions{
		Data:             []byte(`{"stock": 9}`),
		ExpectedPosition: &expectedPos,
	})
	require.NoError(t, err)
	require.False(t, res2.Suppressed)

	// CAS update with stale position should fail
	res3, err := broker.Publish(ctx, channel, "item1", centrifuge.MapPublishOptions{
		Data:             []byte(`{"stock": 8}`),
		ExpectedPosition: &expectedPos, // Using old position
	})
	require.NoError(t, err)
	require.True(t, res3.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonPositionMismatch, res3.SuppressReason)
	require.NotNil(t, res3.CurrentPublication)
	require.Equal(t, []byte(`{"stock": 9}`), res3.CurrentPublication.Data)
}

func TestPostgresMapBroker_CleanupMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup metrics test in short mode")
	}

	registry := prometheus.NewRegistry()
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        2 * time.Second,
			}
		},
		Metrics: centrifuge.MetricsConfig{
			RegistererGatherer: registry,
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := fmt.Sprintf("test_cleanup_metrics_%d", time.Now().UnixNano())

	// Publish two entries with short TTL.
	_, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte("data1"),
	})
	require.NoError(t, err)
	_, err = broker.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data: []byte("data2"),
	})
	require.NoError(t, err)

	// Wait for TTL to expire.
	time.Sleep(3 * time.Second)

	// Trigger TTL check directly.
	broker.expireKeys(ctx)

	// Verify keys_removed counter was incremented.
	families, err := registry.Gather()
	require.NoError(t, err)
	var removedCount float64
	for _, f := range families {
		if f.GetName() == "centrifuge_map_broker_cleanup_keys_removed_count" {
			for _, m := range f.GetMetric() {
				removedCount += m.GetCounter().GetValue()
			}
		}
	}
	require.GreaterOrEqual(t, removedCount, float64(2), "cleanup_keys_removed_count should be at least 2")
}

// TestPostgresMapBroker_KeyTTL tests key TTL (this is a slower test).
func TestPostgresMapBroker_KeyTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TTL test in short mode")
	}

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        2 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := fmt.Sprintf("test_key_ttl_%d", time.Now().UnixNano())

	// Publish with short TTL (2s from channel config)
	_, err := broker.Publish(ctx, channel, "ephemeral", centrifuge.MapPublishOptions{
		Data: []byte("temporary"),
	})
	require.NoError(t, err)

	// Verify key exists
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Key: "ephemeral",
	})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Wait for TTL to expire
	time.Sleep(3 * time.Second)

	// Trigger TTL check
	broker.expireKeys(ctx)

	// Key should be gone
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Key: "ephemeral",
	})
	entries, _, _ = stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Empty(t, entries)
}

// TestPostgresMapBroker_Version tests version-based ordering.
func TestPostgresMapBroker_Version(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_version"

	// Publish with version 2
	res1, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:    []byte("data_v2"),
		Version: 2,
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)

	// Try to publish older version (should be suppressed)
	res2, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:    []byte("data_v1"),
		Version: 1,
	})
	require.NoError(t, err)
	require.True(t, res2.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonVersion, res2.SuppressReason)

	// Publish newer version
	res3, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:    []byte("data_v3"),
		Version: 3,
	})
	require.NoError(t, err)
	require.False(t, res3.Suppressed)

	// Verify state has v3 data
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Key: "key1",
	})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Equal(t, []byte("data_v3"), entries[0].Data)
}

// TestPostgresMapBroker_PerKeyVersion tests that version tracking is per-key independent.
func TestPostgresMapBroker_PerKeyVersion(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_per_key_version"

	// key1 with version=10 → accepted
	res1, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:    []byte("key1_v10"),
		Version: 10,
	})
	require.NoError(t, err)
	require.False(t, res1.Suppressed)

	// key2 with version=5 → accepted (independent, was broken before per-key version)
	res2, err := broker.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data:    []byte("key2_v5"),
		Version: 5,
	})
	require.NoError(t, err)
	require.False(t, res2.Suppressed, "key2 should not be suppressed by key1's version")

	// key1 with version=5 → suppressed (same key, lower version)
	res3, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:    []byte("key1_v5"),
		Version: 5,
	})
	require.NoError(t, err)
	require.True(t, res3.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonVersion, res3.SuppressReason)

	// Remove key1
	_, err = broker.Remove(ctx, channel, "key1", centrifuge.MapRemoveOptions{})
	require.NoError(t, err)

	// Publish key1 with version=1 → accepted (version cleared by remove)
	res4, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:    []byte("key1_v1"),
		Version: 1,
	})
	require.NoError(t, err)
	require.False(t, res4.Suppressed, "key1 version should be cleared after remove")

	// Verify final state
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	require.NoError(t, err)
	state := stateToMapPostgres(stateRes.Publications)
	require.Equal(t, []byte("key1_v1"), state["key1"])
	require.Equal(t, []byte("key2_v5"), state["key2"])
}

// TestPostgresMapBroker_Remove tests removing keys.
func TestPostgresMapBroker_Remove(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_unpublish"

	// Publish some keys
	_, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte("data1"),
	})
	require.NoError(t, err)

	_, err = broker.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data: []byte("data2"),
	})
	require.NoError(t, err)

	// Verify state has 2 keys
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// Remove key1
	res, err := broker.Remove(ctx, channel, "key1", centrifuge.MapRemoveOptions{})
	require.NoError(t, err)
	require.False(t, res.Suppressed)

	// Verify state has 1 key
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	entries, _, _ = stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "key2", entries[0].Key)

	// Remove non-existent key should be suppressed
	res, err = broker.Remove(ctx, channel, "nonexistent", centrifuge.MapRemoveOptions{})
	require.NoError(t, err)
	require.True(t, res.Suppressed)
	require.Equal(t, centrifuge.SuppressReasonKeyNotFound, res.SuppressReason)

	// Verify stream has removal event
	streamResult, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{
			Limit: -1,
		},
	})
	require.NoError(t, err)
	require.Len(t, streamResult.Publications, 3) // key1, key2, remove(key1)
	require.True(t, streamResult.Publications[2].Removed)
	require.Equal(t, "key1", streamResult.Publications[2].Key)
}

// TestPostgresMapBroker_Stats tests state statistics.
func TestPostgresMapBroker_Stats(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_stats"

	// Initially empty
	stats, err := broker.Stats(ctx, channel)
	require.NoError(t, err)
	require.Equal(t, 0, stats.NumKeys)

	// Publish some keys
	for i := 0; i < 5; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte(fmt.Sprintf("data%d", i)),
		})
		require.NoError(t, err)
	}

	// Should have 5 keys
	stats, err = broker.Stats(ctx, channel)
	require.NoError(t, err)
	require.Equal(t, 5, stats.NumKeys)
}

// TestPostgresMapBroker_EpochMismatch tests epoch validation.
func TestPostgresMapBroker_EpochMismatch(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_epoch_mismatch"

	// Client tries to read with old epoch (channel doesn't exist yet)
	_, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Revision: &centrifuge.StreamPosition{
			Epoch:  "old_epoch",
			Offset: 100,
		},
		Limit: 100,
	})
	require.ErrorIs(t, err, centrifuge.ErrorUnrecoverablePosition)

	// Create channel
	_, err = broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte("data1"),
	})
	require.NoError(t, err)

	// Read actual epoch
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	_, pos, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.NotEmpty(t, pos.Epoch)

	// Read with wrong epoch should fail
	_, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Revision: &centrifuge.StreamPosition{
			Epoch:  "wrong_epoch",
			Offset: 1,
		},
		Limit: 100,
	})
	require.ErrorIs(t, err, centrifuge.ErrorUnrecoverablePosition)

	// Read with correct epoch should succeed
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Revision: &pos,
		Limit:    100,
	})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

// TestPostgresMapBroker_ConcurrentPublishOrdering tests that concurrent publishes maintain per-channel ordering.
func TestPostgresMapBroker_ConcurrentPublishOrdering(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_concurrent_order"

	const numGoroutines = 5
	const publishesPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent publishes
	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < publishesPerGoroutine; i++ {
				key := fmt.Sprintf("g%d_k%d", goroutineID, i)
				data := []byte(fmt.Sprintf("data_%d_%d", goroutineID, i))
				_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
					Data: data,
				})
				require.NoError(t, err)
			}
		}(g)
	}

	wg.Wait()

	// Read stream and verify offsets are sequential with no gaps
	streamResult, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{
			Limit: -1,
		},
	})
	require.NoError(t, err)

	expectedCount := numGoroutines * publishesPerGoroutine
	require.Len(t, streamResult.Publications, expectedCount)
	require.Equal(t, uint64(expectedCount), streamResult.Position.Offset)

	// Verify offsets are 1, 2, 3, ... with no gaps
	for i, pub := range streamResult.Publications {
		require.Equal(t, uint64(i+1), pub.Offset, "offset at index %d should be %d, got %d", i, i+1, pub.Offset)
	}
}

// ============================================================================
// Outbox Mode Tests
// ============================================================================

// TestPostgresMapBroker_OutboxOrdering tests that publications are delivered in channel_offset order.
func TestPostgresMapBroker_OutboxOrdering(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, err := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node.Run())

	// Create broker with outbox mode (default)
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)
	require.NoError(t, broker.EnsureSchema(ctx))

	// Use unique channel name per test run
	channel := fmt.Sprintf("test_outbox_order_%d", time.Now().UnixNano())

	// Clean up tables
	cleanupTestTables(ctx, broker)

	t.Cleanup(func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	var received []*centrifuge.Publication
	var receivedMu sync.Mutex
	doneCh := make(chan struct{})

	const numMessages = 10

	err = broker.RegisterEventHandler(&testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			if ch != channel {
				return nil
			}
			receivedMu.Lock()
			received = append(received, pub)
			if len(received) >= numMessages {
				select {
				case <-doneCh:
				default:
					close(doneCh)
				}
			}
			receivedMu.Unlock()
			return nil
		},
	})
	require.NoError(t, err)

	// Give outbox worker a moment to start polling.
	time.Sleep(50 * time.Millisecond)

	// Publish multiple messages in sequence
	for i := 0; i < numMessages; i++ {
		_, err = broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte(fmt.Sprintf("data%d", i)),
		})
		require.NoError(t, err)
	}

	// Wait for all messages
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for all publications")
	}

	// Verify ordering by offset
	receivedMu.Lock()
	defer receivedMu.Unlock()

	require.Len(t, received, numMessages)

	// Offsets should be sequential (1, 2, 3, ...)
	for i, pub := range received {
		require.Equal(t, uint64(i+1), pub.Offset, "offset at index %d should be %d", i, i+1)
	}
}

// TestPostgresMapBroker_OutboxConcurrentPublish tests concurrent publishes maintain ordering.
func TestPostgresMapBroker_OutboxConcurrentPublish(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, err := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node.Run())

	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
		},
	})
	require.NoError(t, err)
	require.NoError(t, broker.EnsureSchema(ctx))

	channel := fmt.Sprintf("test_outbox_concurrent_%d", time.Now().UnixNano())
	cleanupTestTables(ctx, broker)

	t.Cleanup(func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	const numGoroutines = 5
	const publishesPerGoroutine = 10
	totalPublishes := numGoroutines * publishesPerGoroutine

	var received []*centrifuge.Publication
	var receivedMu sync.Mutex
	doneCh := make(chan struct{})

	err = broker.RegisterEventHandler(&testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			if ch != channel {
				return nil
			}
			receivedMu.Lock()
			received = append(received, pub)
			if len(received) >= totalPublishes {
				select {
				case <-doneCh:
				default:
					close(doneCh)
				}
			}
			receivedMu.Unlock()
			return nil
		},
	})
	require.NoError(t, err)

	// Give outbox worker a moment to start polling.
	time.Sleep(50 * time.Millisecond)

	// Concurrent publishes
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < publishesPerGoroutine; i++ {
				key := fmt.Sprintf("g%d_k%d", goroutineID, i)
				data := []byte(fmt.Sprintf("data_%d_%d", goroutineID, i))
				_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
					Data: data,
				})
				require.NoError(t, err)
			}
		}(g)
	}

	wg.Wait()

	// Wait for all messages
	select {
	case <-doneCh:
	case <-time.After(15 * time.Second):
		receivedMu.Lock()
		t.Fatalf("timeout waiting for publications, received %d/%d", len(received), totalPublishes)
		receivedMu.Unlock()
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()

	require.Len(t, received, totalPublishes)

	// Collect offsets and verify no gaps
	offsets := make(map[uint64]bool)
	for _, pub := range received {
		offsets[pub.Offset] = true
	}

	for i := 1; i <= totalPublishes; i++ {
		require.True(t, offsets[uint64(i)], "offset %d should exist", i)
	}
}

// TestPostgresMapBroker_Delta_Outbox tests key-based delta delivery via outbox workers.
func TestPostgresMapBroker_Delta_Outbox(t *testing.T) {
	connString := getPostgresConnString(t)
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})

	type pubEvent struct {
		ch      string
		pub     *centrifuge.Publication
		delta   bool
		prevPub *centrifuge.Publication
	}

	eventCh := make(chan pubEvent, 10)

	handler := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			eventCh <- pubEvent{ch: ch, pub: pub, delta: delta, prevPub: prevPub}
			return nil
		},
	}

	e, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	err = e.RegisterEventHandler(handler)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	// Give outbox worker a moment to start polling.
	time.Sleep(50 * time.Millisecond)

	channel := fmt.Sprintf("test_delta_%d", time.Now().UnixNano())

	waitEvent := func(t *testing.T) pubEvent {
		t.Helper()
		for {
			select {
			case ev := <-eventCh:
				if ev.ch == channel {
					return ev
				}
				// Skip events from other channels (stale outbox entries).
			case <-time.After(10 * time.Second):
				t.Fatal("timeout waiting for publication event")
				return pubEvent{}
			}
		}
	}

	// 1. First publish with UseDelta - no previous state.
	_, err = e.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:     []byte("data1"),
		UseDelta: true,
	})
	require.NoError(t, err)

	ev := waitEvent(t)
	require.False(t, ev.delta, "no previous data means useDelta is false in outbox delivery")
	require.Nil(t, ev.prevPub, "no previous state for first publish")
	require.Equal(t, []byte("data1"), ev.pub.Data)

	// 2. Second publish same key - should get prevPub with first data.
	_, err = e.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:     []byte("data1_updated"),
		UseDelta: true,
	})
	require.NoError(t, err)

	ev = waitEvent(t)
	require.True(t, ev.delta)
	require.NotNil(t, ev.prevPub)
	require.Equal(t, []byte("data1"), ev.prevPub.Data)
	require.Equal(t, []byte("data1_updated"), ev.pub.Data)

	// 3. Different key - no previous state for this key.
	_, err = e.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data:     []byte("data2"),
		UseDelta: true,
	})
	require.NoError(t, err)

	ev = waitEvent(t)
	require.False(t, ev.delta, "no previous data for key2")
	require.Nil(t, ev.prevPub, "no previous state for key2")
	require.Equal(t, []byte("data2"), ev.pub.Data)

	// 4. StreamData present - no delta.
	_, err = e.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:       []byte("data1_full"),
		StreamData: []byte("data1_stream"),
		UseDelta:   true,
	})
	require.NoError(t, err)

	ev = waitEvent(t)
	require.False(t, ev.delta, "StreamData disables delta")
	require.Nil(t, ev.prevPub, "StreamData disables key-based delta")

	// 5. UseDelta=false - no delta.
	_, err = e.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data:     []byte("data2_updated"),
		UseDelta: false,
	})
	require.NoError(t, err)

	ev = waitEvent(t)
	require.False(t, ev.delta)
	require.Nil(t, ev.prevPub, "UseDelta=false means no delta")

	// 6. Third publish to key1 after StreamData update - prevPub should have data1_full.
	_, err = e.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:     []byte("data1_v3"),
		UseDelta: true,
	})
	require.NoError(t, err)

	ev = waitEvent(t)
	require.True(t, ev.delta)
	require.NotNil(t, ev.prevPub)
	require.Equal(t, []byte("data1_full"), ev.prevPub.Data, "prevPub should have state data, not stream data")
}

func TestPostgresMapBroker_Clear(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := "test_clear"

	// Publish some keyed state and stream entries.
	for i := 0; i < 3; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte(fmt.Sprintf("data%d", i)),
		})
		require.NoError(t, err)
	}

	// Verify data exists.
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{Limit: 100})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 3)

	streamResult, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{Filter: centrifuge.StreamFilter{Limit: -1}})
	require.NoError(t, err)
	require.Len(t, streamResult.Publications, 3)

	stats, err := broker.Stats(ctx, channel)
	require.NoError(t, err)
	require.Equal(t, 3, stats.NumKeys)

	// Clear the channel.
	err = broker.Clear(ctx, channel, centrifuge.MapClearOptions{})
	require.NoError(t, err)

	// State should be empty.
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{Limit: 100})
	entries, _, _ = stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Empty(t, entries)

	// Stream should be empty.
	streamResult, err = broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{Filter: centrifuge.StreamFilter{Limit: -1}})
	require.NoError(t, err)
	require.Empty(t, streamResult.Publications)

	// Stats should show zero keys.
	stats, err = broker.Stats(ctx, channel)
	require.NoError(t, err)
	require.Equal(t, 0, stats.NumKeys)
}

func TestPostgresMapBroker_ClearDoesNotAffectOtherChannels(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()

	// Populate two channels.
	for _, ch := range []string{"test_clear_iso_ch1", "test_clear_iso_ch2"} {
		_, err := broker.Publish(ctx, ch, "k", centrifuge.MapPublishOptions{
			Data: []byte("v"),
		})
		require.NoError(t, err)
	}

	// Clear only ch1.
	err := broker.Clear(ctx, "test_clear_iso_ch1", centrifuge.MapClearOptions{})
	require.NoError(t, err)

	// ch1 empty.
	stateRes, err := broker.ReadState(ctx, "test_clear_iso_ch1", centrifuge.MapReadStateOptions{Limit: 100})
	entries, _, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Empty(t, entries)

	// ch2 still intact.
	stateRes, err = broker.ReadState(ctx, "test_clear_iso_ch2", centrifuge.MapReadStateOptions{Limit: 100})
	entries, _, _ = stateRes.Publications, stateRes.Position, stateRes.Cursor
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

// ============================================================================
// EnsureSchema Tests
// ============================================================================

// dropAllSchemaObjects drops all centrifuge map objects (both JSONB and binary variants)
// in reverse dependency order.
func dropAllSchemaObjects(ctx context.Context, pool *pgxpool.Pool) {
	for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
		// Drop functions (they depend on tables).
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %spublish CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %spublish_strict CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %sremove CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %sremove_strict CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %sexpire_keys CASCADE", prefix))

		// Drop tables.
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sidempotency CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sstream CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sstate CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %smeta CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sshard_lock CASCADE", prefix))
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %sschema_version CASCADE", prefix))
	}
}

// verifySchemaComplete checks that all expected tables, indexes, functions, and column types exist.
// prefix is "cf_map_" or "cf_binary_map_".
func verifySchemaComplete(t *testing.T, ctx context.Context, pool *pgxpool.Pool, prefix string, expectJSONB bool) {
	t.Helper()

	// Check tables exist.
	for _, suffix := range []string{"stream", "state", "meta", "idempotency", "shard_lock", "schema_version"} {
		table := prefix + suffix
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, table).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %s should exist", table)
	}

	// Check indexes exist.
	for _, suffix := range []string{
		"stream_channel_offset_idx",
		"stream_channel_id_idx",
		"stream_created_at_idx",
		"stream_shard_id_idx",
		"state_ordered_idx",
		"state_expires_idx",
		"meta_expires_idx",
		"idempotency_expires_idx",
	} {
		idx := prefix + suffix
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "index %s should exist", idx)
	}

	// Check functions exist.
	for _, suffix := range []string{"publish", "publish_strict", "remove", "remove_strict", "expire_keys"} {
		fn := prefix + suffix
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_proc WHERE proname = $1)`, fn).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "function %s should exist", fn)
	}

	// Check data column types.
	expectedType := "jsonb"
	if !expectJSONB {
		expectedType = "bytea"
	}

	dataColumns := []struct {
		table  string
		column string
	}{
		{prefix + "stream", "data"},
		{prefix + "stream", "previous_data"},
		{prefix + "stream", "conn_info"},
		{prefix + "stream", "chan_info"},
		{prefix + "state", "data"},
		{prefix + "state", "conn_info"},
		{prefix + "state", "chan_info"},
	}
	for _, dc := range dataColumns {
		var dataType string
		err := pool.QueryRow(ctx,
			`SELECT data_type FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`,
			dc.table, dc.column).Scan(&dataType)
		require.NoError(t, err, "column %s.%s should exist", dc.table, dc.column)
		require.Equal(t, expectedType, dataType, "column %s.%s should be %s", dc.table, dc.column, expectedType)
	}

	// tags should always be JSONB regardless of BinaryData setting.
	for _, suffix := range []string{"stream", "state"} {
		table := prefix + suffix
		var dataType string
		err := pool.QueryRow(ctx,
			`SELECT data_type FROM information_schema.columns WHERE table_name = $1 AND column_name = 'tags'`,
			table).Scan(&dataType)
		require.NoError(t, err)
		require.Equal(t, "jsonb", dataType, "%s.tags should always be jsonb", table)
	}
}

// TestPostgresMapBroker_EnsureSchema_Fresh tests creating schema from scratch.
func TestPostgresMapBroker_EnsureSchema_Fresh(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	// Drop everything first.
	dropAllSchemaObjects(ctx, broker.pool)

	// Create from scratch.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Verify all objects exist for both prefixes.
	verifySchemaComplete(t, ctx, broker.pool, "cf_map_", true)
	verifySchemaComplete(t, ctx, broker.pool, "cf_binary_map_", false)
}

// TestPostgresMapBroker_EnsureSchema_Idempotent tests calling EnsureSchema twice.
func TestPostgresMapBroker_EnsureSchema_Idempotent(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	// First call.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Second call — should succeed without errors.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	verifySchemaComplete(t, ctx, broker.pool, "cf_map_", true)
	verifySchemaComplete(t, ctx, broker.pool, "cf_binary_map_", false)
}

// TestPostgresMapBroker_EnsureSchema_PartialState tests that EnsureSchema handles partial schema.
func TestPostgresMapBroker_EnsureSchema_PartialState(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	// Drop everything.
	dropAllSchemaObjects(ctx, broker.pool)

	// Create only some tables manually.
	_, err = broker.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS cf_map_meta (
		channel TEXT PRIMARY KEY, top_offset BIGINT NOT NULL DEFAULT 0,
		epoch TEXT NOT NULL DEFAULT '', version BIGINT DEFAULT 0,
		version_epoch TEXT, created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW(), expires_at TIMESTAMPTZ
	)`)
	require.NoError(t, err)

	// EnsureSchema should create the rest.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	verifySchemaComplete(t, ctx, broker.pool, "cf_map_", true)
	verifySchemaComplete(t, ctx, broker.pool, "cf_binary_map_", false)
}

// TestPostgresMapBroker_EnsureSchema_BinaryData tests BYTEA columns when BinaryData=true.
func TestPostgresMapBroker_EnsureSchema_BinaryData(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		NumShards:  4,
		BinaryData: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Verify both prefixes exist (EnsureSchema creates both).
	verifySchemaComplete(t, ctx, broker.pool, "cf_map_", true)
	verifySchemaComplete(t, ctx, broker.pool, "cf_binary_map_", false)
}

// TestPostgresMapBroker_EnsureSchema_FunctionalAfterSetup tests that the broker works after EnsureSchema.
func TestPostgresMapBroker_EnsureSchema_FunctionalAfterSetup(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
		},
	})
	require.NoError(t, err)

	// Drop everything and recreate from scratch.
	dropAllSchemaObjects(ctx, broker.pool)

	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	err = broker.RegisterEventHandler(nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	channel := fmt.Sprintf("test_ensure_func_%d", time.Now().UnixNano())

	// Publish.
	res, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte(`{"hello":"world"}`),
	})
	require.NoError(t, err)
	require.False(t, res.Suppressed)
	require.Equal(t, uint64(1), res.Position.Offset)

	// ReadState.
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{Limit: 100})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 1)
	require.Equal(t, "key1", stateRes.Publications[0].Key)
	// JSONB normalizes whitespace, so compare via JSONEq.
	require.JSONEq(t, `{"hello":"world"}`, string(stateRes.Publications[0].Data))

	// Remove.
	removeRes, err := broker.Remove(ctx, channel, "key1", centrifuge.MapRemoveOptions{})
	require.NoError(t, err)
	require.False(t, removeRes.Suppressed)

	// Verify removed.
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{Limit: 100})
	require.NoError(t, err)
	require.Empty(t, stateRes.Publications)
}

// TestPostgresMapBroker_EnsureSchema_VersionTracking tests that schema version is tracked.
func TestPostgresMapBroker_EnsureSchema_VersionTracking(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Verify version is set in both prefix tables.
	for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
		var version int
		err := broker.pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT schema_version FROM %sschema_version WHERE id = 1`, prefix),
		).Scan(&version)
		require.NoError(t, err, "schema_version should exist for prefix %s", prefix)
		require.Equal(t, schemaVersion, version, "version should match schemaVersion for prefix %s", prefix)
	}
}

// TestPostgresMapBroker_EnsureSchema_BothPrefixesCreated tests that EnsureSchema creates both variants.
func TestPostgresMapBroker_EnsureSchema_BothPrefixesCreated(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	// BinaryData=false, but both should still be created.
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Verify both prefixes have all tables, indexes, and functions.
	verifySchemaComplete(t, ctx, broker.pool, "cf_map_", true)
	verifySchemaComplete(t, ctx, broker.pool, "cf_binary_map_", false)

	// Verify shard_lock populated for both prefixes.
	for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
		var count int
		err := broker.pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM %sshard_lock`, prefix),
		).Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 4, count, "shard_lock should have 4 rows for prefix %s", prefix)
	}
}

// TestPostgresMapBroker_EnsureSchema_MigrationExecution tests that migrations run correctly.
func TestPostgresMapBroker_EnsureSchema_MigrationExecution(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
		// Cleanup: drop test columns and restore version.
		for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
			_, _ = broker.pool.Exec(ctx, fmt.Sprintf(
				`ALTER TABLE %sstate DROP COLUMN IF EXISTS test_col`, prefix))
		}
		schemaVersion = 1
		delete(schemaMigrations, 2)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	// Create v1 schema.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Register migration v2: add test_col to both prefixes.
	schemaMigrations[2] = `
		ALTER TABLE cf_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
		ALTER TABLE cf_binary_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
	`
	schemaVersion = 2

	// Run EnsureSchema again — should apply migration.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Verify test_col exists on both tables.
	for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
		var exists bool
		err := broker.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = 'test_col')`,
			prefix+"state",
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "test_col should exist on %sstate", prefix)
	}

	// Verify version is now 2.
	var version int
	err = broker.pool.QueryRow(ctx,
		`SELECT schema_version FROM cf_map_schema_version WHERE id = 1`,
	).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, 2, version)
}

// TestPostgresMapBroker_EnsureSchema_MigrationIdempotent tests that migrations can run twice.
func TestPostgresMapBroker_EnsureSchema_MigrationIdempotent(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
		for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
			_, _ = broker.pool.Exec(ctx, fmt.Sprintf(
				`ALTER TABLE %sstate DROP COLUMN IF EXISTS test_col`, prefix))
		}
		schemaVersion = 1
		delete(schemaMigrations, 2)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	// Create v1 schema.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Register migration v2.
	schemaMigrations[2] = `
		ALTER TABLE cf_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
		ALTER TABLE cf_binary_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
	`
	schemaVersion = 2

	// First migration run.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Reset version in DB to force re-run of DDL + migration.
	_, _ = broker.pool.Exec(ctx, `UPDATE cf_map_schema_version SET schema_version = 1 WHERE id = 1`)
	_, _ = broker.pool.Exec(ctx, `UPDATE cf_binary_map_schema_version SET schema_version = 1 WHERE id = 1`)

	// Second migration run — should succeed (idempotent).
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)
}

// TestPostgresMapBroker_EnsureSchema_FreshInstallSkipsMigrations tests that fresh installs skip migrations.
func TestPostgresMapBroker_EnsureSchema_FreshInstallSkipsMigrations(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
		for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
			_, _ = broker.pool.Exec(ctx, fmt.Sprintf(
				`ALTER TABLE %sstate DROP COLUMN IF EXISTS test_col`, prefix))
		}
		schemaVersion = 1
		delete(schemaMigrations, 2)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	// Set up v2 with a migration that adds a column (harmless but verifiable).
	schemaMigrations[2] = `
		ALTER TABLE cf_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
		ALTER TABLE cf_binary_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
	`
	schemaVersion = 2

	// Fresh install — DDL creates latest schema, migration should be skipped.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Version should be 2 (set by UPDATE at end).
	var version int
	err = broker.pool.QueryRow(ctx,
		`SELECT schema_version FROM cf_map_schema_version WHERE id = 1`,
	).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, 2, version)
}

// TestPostgresMapBroker_EnsureSchema_VersionPreservedOnDDLRerun tests that DO NOTHING preserves version.
func TestPostgresMapBroker_EnsureSchema_VersionPreservedOnDDLRerun(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
		schemaVersion = 1
	})

	dropAllSchemaObjects(ctx, broker.pool)

	// Create v1 schema.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Manually bump version to 2 (simulating a previous upgrade).
	_, err = broker.pool.Exec(ctx, `UPDATE cf_map_schema_version SET schema_version = 2 WHERE id = 1`)
	require.NoError(t, err)
	_, err = broker.pool.Exec(ctx, `UPDATE cf_binary_map_schema_version SET schema_version = 2 WHERE id = 1`)
	require.NoError(t, err)

	// Set schemaVersion to 2 so fast path matches.
	schemaVersion = 2

	// Call EnsureSchema — should take fast path (version matches, probe OK).
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Verify version is still 2 (not reset to 1 by DDL's DO NOTHING).
	var version int
	err = broker.pool.QueryRow(ctx,
		`SELECT schema_version FROM cf_map_schema_version WHERE id = 1`,
	).Scan(&version)
	require.NoError(t, err)
	require.Equal(t, 2, version)
}

// TestPostgresMapBroker_EnsureSchema_FunctionalAfterMigration tests that all operations work after migration.
func TestPostgresMapBroker_EnsureSchema_FunctionalAfterMigration(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:       connString,
		NumShards: 4,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = broker.Close(ctx)
		_ = node.Shutdown(ctx)
		for _, prefix := range []string{"cf_map_", "cf_binary_map_"} {
			_, _ = broker.pool.Exec(ctx, fmt.Sprintf(
				`ALTER TABLE %sstate DROP COLUMN IF EXISTS test_col`, prefix))
		}
		schemaVersion = 1
		delete(schemaMigrations, 2)
	})

	dropAllSchemaObjects(ctx, broker.pool)

	// Create v1 schema.
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	// Register and apply v2 migration.
	schemaMigrations[2] = `
		ALTER TABLE cf_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
		ALTER TABLE cf_binary_map_state ADD COLUMN IF NOT EXISTS test_col TEXT;
	`
	schemaVersion = 2
	err = broker.EnsureSchema(ctx)
	require.NoError(t, err)

	err = broker.RegisterEventHandler(nil)
	require.NoError(t, err)

	channel := fmt.Sprintf("test_migration_func_%d", time.Now().UnixNano())

	// Publish.
	res, err := broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data: []byte(`{"hello":"world"}`),
	})
	require.NoError(t, err)
	require.False(t, res.Suppressed)

	// ReadState.
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{Limit: 100})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 1)
	require.Equal(t, "key1", stateRes.Publications[0].Key)

	// ReadStream.
	streamRes, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{Limit: -1},
	})
	require.NoError(t, err)
	require.Len(t, streamRes.Publications, 1)
}

// TestPostgresMapBroker_OrderedStateAsc tests that ASC ordering returns entries
// in ascending score order (lowest score first).
func TestPostgresMapBroker_OrderedStateAsc(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Ordered:       true,
				StreamSize:    100,
				StreamTTL:     300 * time.Second,
				KeyTTL:        300 * time.Second,
				Mode: centrifuge.MapModeDurable,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := fmt.Sprintf("test_ordered_asc_%d", time.Now().UnixNano())

	testCases := []struct {
		key   string
		score int64
		data  string
	}{
		{"key_c", 30, "data_c"},
		{"key_a", 10, "data_a"},
		{"key_e", 50, "data_e"},
		{"key_b", 20, "data_b"},
		{"key_d", 40, "data_d"},
	}

	for _, tc := range testCases {
		_, err := broker.Publish(ctx, channel, tc.key, centrifuge.MapPublishOptions{
			Data:  []byte(tc.data),
			Score: tc.score,
		})
		require.NoError(t, err)
	}

	// ASC: lowest score first.
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
		Asc:   true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 5)

	expectedAsc := []string{"key_a", "key_b", "key_c", "key_d", "key_e"}
	for i, entry := range stateRes.Publications {
		require.Equal(t, expectedAsc[i], entry.Key, "ASC entry %d", i)
	}

	// DESC (default): highest score first.
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 5)

	expectedDesc := []string{"key_e", "key_d", "key_c", "key_b", "key_a"}
	for i, entry := range stateRes.Publications {
		require.Equal(t, expectedDesc[i], entry.Key, "DESC entry %d", i)
	}
}

// TestPostgresMapBroker_OrderedStatePaginationAsc tests cursor-based pagination
// with ASC ordering across multiple pages.
func TestPostgresMapBroker_OrderedStatePaginationAsc(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Ordered:       true,
				StreamSize:    100,
				StreamTTL:     300 * time.Second,
				KeyTTL:        300 * time.Second,
				Mode: centrifuge.MapModeDurable,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := fmt.Sprintf("test_ordered_pagination_asc_%d", time.Now().UnixNano())

	// Publish 10 entries with scores 100..1000.
	for i := 1; i <= 10; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("key_%02d", i), centrifuge.MapPublishOptions{
			Data:  []byte(fmt.Sprintf("data_%02d", i)),
			Score: int64(i * 100),
		})
		require.NoError(t, err)
	}

	// Page 1.
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 3, Asc: true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 3)
	require.NotEmpty(t, stateRes.Cursor)
	require.Equal(t, "key_01", stateRes.Publications[0].Key)
	require.Equal(t, "key_02", stateRes.Publications[1].Key)
	require.Equal(t, "key_03", stateRes.Publications[2].Key)

	// Page 2.
	stateRes2, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Cursor: stateRes.Cursor, Limit: 3, Asc: true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes2.Publications, 3)
	require.NotEmpty(t, stateRes2.Cursor)
	require.Equal(t, "key_04", stateRes2.Publications[0].Key)
	require.Equal(t, "key_05", stateRes2.Publications[1].Key)
	require.Equal(t, "key_06", stateRes2.Publications[2].Key)

	// Page 3.
	stateRes3, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Cursor: stateRes2.Cursor, Limit: 3, Asc: true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes3.Publications, 3)
	require.NotEmpty(t, stateRes3.Cursor)
	require.Equal(t, "key_07", stateRes3.Publications[0].Key)
	require.Equal(t, "key_08", stateRes3.Publications[1].Key)
	require.Equal(t, "key_09", stateRes3.Publications[2].Key)

	// Page 4: last entry.
	stateRes4, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Cursor: stateRes3.Cursor, Limit: 3, Asc: true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes4.Publications, 1)
	require.Empty(t, stateRes4.Cursor, "No more pages")
	require.Equal(t, "key_10", stateRes4.Publications[0].Key)
}

// TestPostgresMapBroker_OrderedStateAscSameScores tests ASC ordering with
// same-score entries — secondary sort by key ascending.
func TestPostgresMapBroker_OrderedStateAscSameScores(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Ordered:       true,
				StreamSize:    100,
				StreamTTL:     300 * time.Second,
				KeyTTL:        300 * time.Second,
				Mode: centrifuge.MapModeDurable,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	channel := fmt.Sprintf("test_ordered_asc_same_scores_%d", time.Now().UnixNano())

	// All entries have score=100.
	for _, key := range []string{"zebra", "apple", "mango", "banana"} {
		_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
			Data:  []byte("data"),
			Score: 100,
		})
		require.NoError(t, err)
	}

	// ASC with same scores → key ASC.
	stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100, Asc: true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 4)
	require.Equal(t, "apple", stateRes.Publications[0].Key)
	require.Equal(t, "banana", stateRes.Publications[1].Key)
	require.Equal(t, "mango", stateRes.Publications[2].Key)
	require.Equal(t, "zebra", stateRes.Publications[3].Key)

	// DESC with same scores → key DESC.
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 100,
	})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 4)
	require.Equal(t, "zebra", stateRes.Publications[0].Key)
	require.Equal(t, "mango", stateRes.Publications[1].Key)
	require.Equal(t, "banana", stateRes.Publications[2].Key)
	require.Equal(t, "apple", stateRes.Publications[3].Key)

	// Paginate ASC with limit=2.
	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: 2, Asc: true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 2)
	require.NotEmpty(t, stateRes.Cursor)
	require.Equal(t, "apple", stateRes.Publications[0].Key)
	require.Equal(t, "banana", stateRes.Publications[1].Key)

	stateRes, err = broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Cursor: stateRes.Cursor, Limit: 2, Asc: true,
	})
	require.NoError(t, err)
	require.Len(t, stateRes.Publications, 2)
	require.Empty(t, stateRes.Cursor)
	require.Equal(t, "mango", stateRes.Publications[0].Key)
	require.Equal(t, "zebra", stateRes.Publications[1].Key)
}

func TestPostgresMapBroker_ClientInfoInState(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	info := &centrifuge.ClientInfo{
		ClientID: "c1",
		UserID:   "u1",
		ConnInfo: []byte("conn"),
		ChanInfo: []byte("chan"),
	}

	t.Run("ReadState_with_key_filter", func(t *testing.T) {
		ch := "client_info_state_key_ch"
		_, err := broker.Publish(ctx, ch, "k1", centrifuge.MapPublishOptions{
			Data:       []byte("data1"),
			ClientInfo: info,
		})
		require.NoError(t, err)
		result, err := broker.ReadState(ctx, ch, centrifuge.MapReadStateOptions{
			Key: "k1",
		})
		require.NoError(t, err)
		require.Len(t, result.Publications, 1)
		pub := result.Publications[0]
		require.NotNil(t, pub.Info, "ClientInfo should be present in state")
		require.Equal(t, "c1", pub.Info.ClientID)
		require.Equal(t, "u1", pub.Info.UserID)
		require.Equal(t, []byte("conn"), pub.Info.ConnInfo)
		require.Equal(t, []byte("chan"), pub.Info.ChanInfo)
	})

	t.Run("ReadState_paginated", func(t *testing.T) {
		ch := "client_info_state_pag_ch"
		_, err := broker.Publish(ctx, ch, "k1", centrifuge.MapPublishOptions{
			Data:       []byte("data1"),
			ClientInfo: info,
		})
		require.NoError(t, err)
		result, err := broker.ReadState(ctx, ch, centrifuge.MapReadStateOptions{
			Limit: -1,
		})
		require.NoError(t, err)
		require.Len(t, result.Publications, 1)
		pub := result.Publications[0]
		require.NotNil(t, pub.Info, "ClientInfo should be present in paginated state")
		require.Equal(t, "c1", pub.Info.ClientID)
		require.Equal(t, "u1", pub.Info.UserID)
		require.Equal(t, []byte("conn"), pub.Info.ConnInfo)
		require.Equal(t, []byte("chan"), pub.Info.ChanInfo)
	})
}

func TestPostgresMapBroker_ClientInfoInStream(t *testing.T) {
	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	broker := newTestPostgresMapBroker(t, node)

	ctx := context.Background()
	info := &centrifuge.ClientInfo{
		ClientID: "c1",
		UserID:   "u1",
		ConnInfo: []byte("conn"),
		ChanInfo: []byte("chan"),
	}

	t.Run("ReadStream_contains_client_info", func(t *testing.T) {
		ch := fmt.Sprintf("client_info_stream_ch_%d", time.Now().UnixNano())
		_, err := broker.Publish(ctx, ch, "k1", centrifuge.MapPublishOptions{
			Data:       []byte("data1"),
			ClientInfo: info,
		})
		require.NoError(t, err)
		result, err := broker.ReadStream(ctx, ch, centrifuge.MapReadStreamOptions{
			Filter: centrifuge.StreamFilter{Limit: -1},
		})
		require.NoError(t, err)
		require.Len(t, result.Publications, 1)
		pub := result.Publications[0]
		require.NotNil(t, pub.Info, "ClientInfo should be present in stream")
		require.Equal(t, "c1", pub.Info.ClientID)
		require.Equal(t, "u1", pub.Info.UserID)
		require.Equal(t, []byte("conn"), pub.Info.ConnInfo)
		require.Equal(t, []byte("chan"), pub.Info.ChanInfo)
	})
}

// TestPostgresMapBroker_ClientInfoDelivery_Outbox tests that ClientInfo is delivered
// via outbox workers (single-node, local delivery).
func TestPostgresMapBroker_ClientInfoDelivery_Outbox(t *testing.T) {
	connString := getPostgresConnString(t)
	node, err := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node.Run())

	type pubEvent struct {
		ch  string
		pub *centrifuge.Publication
	}

	eventCh := make(chan pubEvent, 10)

	handler := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			eventCh <- pubEvent{ch: ch, pub: pub}
			return nil
		},
	}

	e, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	err = e.RegisterEventHandler(handler)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	// Give outbox worker a moment to start polling.
	time.Sleep(50 * time.Millisecond)

	channel := fmt.Sprintf("test_client_info_outbox_%d", time.Now().UnixNano())

	info := &centrifuge.ClientInfo{
		ClientID: "c1",
		UserID:   "u1",
		ConnInfo: []byte("conn"),
		ChanInfo: []byte("chan"),
	}

	_, err = e.Publish(ctx, channel, "k1", centrifuge.MapPublishOptions{
		Data:       []byte("data1"),
		ClientInfo: info,
	})
	require.NoError(t, err)

	for {
		select {
		case ev := <-eventCh:
			if ev.ch != channel {
				continue
			}
			require.NotNil(t, ev.pub.Info, "ClientInfo should be present in outbox delivery")
			require.Equal(t, "c1", ev.pub.Info.ClientID)
			require.Equal(t, "u1", ev.pub.Info.UserID)
			require.Equal(t, []byte("conn"), ev.pub.Info.ConnInfo)
			require.Equal(t, []byte("chan"), ev.pub.Info.ChanInfo)
			return
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for publication event")
		}
	}
}

// TestPostgresMapBroker_AllColumnTypes verifies that every Publication field
// is correctly parsed from PostgreSQL across all three read paths: ReadState,
// ReadStream, and outbox delivery (HandlePublication). This catches wire-format
// mismatches (binary vs text) for all column types.
func TestPostgresMapBroker_AllColumnTypes(t *testing.T) {
	connString := getPostgresConnString(t)
	node, err := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
				Ordered:       true,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node.Run())

	type pubEvent struct {
		ch  string
		pub *centrifuge.Publication
	}
	eventCh := make(chan pubEvent, 10)

	handler := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			eventCh <- pubEvent{ch: ch, pub: pub}
			return nil
		},
	}

	e, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	err = e.RegisterEventHandler(handler)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	time.Sleep(50 * time.Millisecond)

	channel := fmt.Sprintf("test_all_cols_%d", time.Now().UnixNano())

	info := &centrifuge.ClientInfo{
		ClientID: "client1",
		UserID:   "user1",
		ConnInfo: []byte(`{"ip":"1.2.3.4"}`),
		ChanInfo: []byte(`{"role":"admin"}`),
	}
	tags := map[string]string{"sector": "tech", "region": "us"}

	// Publish with all fields populated.
	_, err = e.Publish(ctx, channel, "k1", centrifuge.MapPublishOptions{
		Data:       []byte(`{"price":100}`),
		Tags:       tags,
		Score:      42,
		ClientInfo: info,
	})
	require.NoError(t, err)

	// Also publish a removal to test Removed flag.
	_, err = e.Remove(ctx, channel, "k1", centrifuge.MapRemoveOptions{})
	require.NoError(t, err)

	// --- Verify ReadStream (has both publish and remove entries) ---
	streamResult, err := e.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
		Filter: centrifuge.StreamFilter{Limit: -1},
	})
	require.NoError(t, err)
	require.Len(t, streamResult.Publications, 2)

	// First stream entry: the publish.
	sp := streamResult.Publications[0]
	require.Equal(t, "k1", sp.Key)
	require.Equal(t, []byte(`{"price":100}`), sp.Data)
	require.Equal(t, tags, sp.Tags)
	require.Equal(t, int64(42), sp.Score)
	require.False(t, sp.Removed)
	require.NotZero(t, sp.Offset)
	require.NotNil(t, sp.Info, "ClientInfo must be present in stream")
	require.Equal(t, "client1", sp.Info.ClientID)
	require.Equal(t, "user1", sp.Info.UserID)
	require.Equal(t, []byte(`{"ip":"1.2.3.4"}`), sp.Info.ConnInfo)
	require.Equal(t, []byte(`{"role":"admin"}`), sp.Info.ChanInfo)

	// Second stream entry: the removal.
	sr := streamResult.Publications[1]
	require.Equal(t, "k1", sr.Key)
	require.True(t, sr.Removed)
	require.NotZero(t, sr.Offset)

	// --- Verify outbox delivery ---
	// Collect outbox events for our channel.
	var outboxPubs []*centrifuge.Publication
	deadline := time.After(10 * time.Second)
	for len(outboxPubs) < 2 {
		select {
		case ev := <-eventCh:
			if ev.ch == channel {
				outboxPubs = append(outboxPubs, ev.pub)
			}
		case <-deadline:
			t.Fatalf("timeout waiting for outbox events, got %d", len(outboxPubs))
		}
	}

	// First outbox event: the publish.
	op := outboxPubs[0]
	require.Equal(t, "k1", op.Key)
	require.Equal(t, []byte(`{"price":100}`), op.Data)
	require.Equal(t, tags, op.Tags)
	require.Equal(t, int64(42), op.Score)
	require.False(t, op.Removed)
	require.NotZero(t, op.Offset)
	require.NotNil(t, op.Info, "ClientInfo must be present in outbox delivery")
	require.Equal(t, "client1", op.Info.ClientID)
	require.Equal(t, "user1", op.Info.UserID)
	require.Equal(t, []byte(`{"ip":"1.2.3.4"}`), op.Info.ConnInfo)
	require.Equal(t, []byte(`{"role":"admin"}`), op.Info.ChanInfo)

	// Second outbox event: the removal.
	or := outboxPubs[1]
	require.Equal(t, "k1", or.Key)
	require.True(t, or.Removed)

	// --- Verify ReadState (key was removed, so state should be empty) ---
	// Publish again to have a key in state for verification.
	_, err = e.Publish(ctx, channel, "k2", centrifuge.MapPublishOptions{
		Data:       []byte(`{"price":200}`),
		Tags:       map[string]string{"sector": "finance"},
		Score:      99,
		ClientInfo: info,
	})
	require.NoError(t, err)

	stateResult, err := e.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
		Limit: -1,
	})
	require.NoError(t, err)
	require.Len(t, stateResult.Publications, 1)

	sk := stateResult.Publications[0]
	require.Equal(t, "k2", sk.Key)
	require.Equal(t, []byte(`{"price":200}`), sk.Data)
	require.Equal(t, map[string]string{"sector": "finance"}, sk.Tags)
	require.Equal(t, int64(99), sk.Score)
	require.NotZero(t, sk.Offset)
	require.NotNil(t, sk.Info, "ClientInfo must be present in state")
	require.Equal(t, "client1", sk.Info.ClientID)
	require.Equal(t, "user1", sk.Info.UserID)
	require.Equal(t, []byte(`{"ip":"1.2.3.4"}`), sk.Info.ConnInfo)
	require.Equal(t, []byte(`{"role":"admin"}`), sk.Info.ChanInfo)
}

// ============================================================================
// Redis Broker Fan-out Tests (advisory lock mode)
// ============================================================================

// newTestRedisBrokerForFanout creates a RedisBroker suitable for PG fan-out testing.
// Does NOT call node.Run() or SetBroker — the PG broker handles registration.
func newTestRedisBrokerForFanout(tb testing.TB, n *centrifuge.Node) *centrifuge.RedisBroker {
	tb.Helper()
	redisConf := centrifuge.RedisShardConfig{
		Address:        "127.0.0.1:6379",
		IOTimeout:      10 * time.Second,
		ConnectTimeout: 10 * time.Second,
	}
	s, err := centrifuge.NewRedisShard(n, redisConf)
	require.NoError(tb, err)

	prefix := "pg_fanout_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	broker, err := centrifuge.NewRedisBroker(n, centrifuge.RedisBrokerConfig{
		Prefix: prefix,
		Shards: []*centrifuge.RedisShard{s},
	})
	require.NoError(tb, err)
	return broker
}

// TestPostgresMapBroker_RedisFanout_Delivery tests that publications reach
// the event handler via Redis PUB/SUB fan-out with advisory locking.
func TestPostgresMapBroker_RedisFanout_Delivery(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, err := centrifuge.New(centrifuge.Config{
		LogLevel:   centrifuge.LogLevelDebug,
		LogHandler: func(entry centrifuge.LogEntry) {},
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node.Run())

	redisBroker := newTestRedisBrokerForFanout(t, node)

	type pubEvent struct {
		ch      string
		pub     *centrifuge.Publication
		sp      centrifuge.StreamPosition
		delta   bool
		prevPub *centrifuge.Publication
	}

	eventCh := make(chan pubEvent, 20)

	handler := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			eventCh <- pubEvent{ch: ch, pub: pub, sp: sp, delta: delta, prevPub: prevPub}
			return nil
		},
	}

	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Broker:     redisBroker,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)
	require.NoError(t, broker.EnsureSchema(ctx))
	cleanupTestTables(ctx, broker)

	err = broker.RegisterEventHandler(handler)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	channel := fmt.Sprintf("test_redis_fanout_%d", time.Now().UnixNano())

	// Subscribe via PG broker (delegates to Redis).
	err = broker.Subscribe(channel)
	require.NoError(t, err)

	// Give workers time to start + subscribe to propagate.
	time.Sleep(200 * time.Millisecond)

	// Publish via PG broker.
	const numMessages = 5
	for i := 0; i < numMessages; i++ {
		_, err = broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte(fmt.Sprintf("data%d", i)),
		})
		require.NoError(t, err)
	}

	// Wait for all messages via Redis PUB/SUB.
	var received []pubEvent
	deadline := time.After(15 * time.Second)
	for len(received) < numMessages {
		select {
		case ev := <-eventCh:
			if ev.ch == channel {
				received = append(received, ev)
			}
		case <-deadline:
			t.Fatalf("timeout waiting for publications, received %d/%d", len(received), numMessages)
		}
	}

	require.Len(t, received, numMessages)

	// Verify fields propagated correctly.
	for _, ev := range received {
		require.NotZero(t, ev.sp.Offset, "offset should be set")
		require.NotEmpty(t, ev.sp.Epoch, "epoch should be set")
		require.NotEmpty(t, ev.pub.Key, "key should be set")
		require.NotEmpty(t, ev.pub.Data, "data should be set")
	}

	// Unsubscribe.
	err = broker.Unsubscribe(channel)
	require.NoError(t, err)
}

// TestPostgresMapBroker_RedisFanout_Delta tests that delta/prevPub is correctly
// propagated through Redis PUB/SUB fan-out.
func TestPostgresMapBroker_RedisFanout_Delta(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, err := centrifuge.New(centrifuge.Config{
		LogLevel:   centrifuge.LogLevelDebug,
		LogHandler: func(entry centrifuge.LogEntry) {},
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node.Run())

	redisBroker := newTestRedisBrokerForFanout(t, node)

	type pubEvent struct {
		ch      string
		pub     *centrifuge.Publication
		delta   bool
		prevPub *centrifuge.Publication
	}

	eventCh := make(chan pubEvent, 20)

	handler := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			eventCh <- pubEvent{ch: ch, pub: pub, delta: delta, prevPub: prevPub}
			return nil
		},
	}

	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Broker:     redisBroker,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)
	require.NoError(t, broker.EnsureSchema(ctx))
	cleanupTestTables(ctx, broker)

	err = broker.RegisterEventHandler(handler)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	channel := fmt.Sprintf("test_redis_delta_%d", time.Now().UnixNano())

	err = broker.Subscribe(channel)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	waitEvent := func(t *testing.T) pubEvent {
		t.Helper()
		for {
			select {
			case ev := <-eventCh:
				if ev.ch == channel {
					return ev
				}
			case <-time.After(15 * time.Second):
				t.Fatal("timeout waiting for publication event")
				return pubEvent{}
			}
		}
	}

	// First publish with UseDelta — no previous state.
	_, err = broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:     []byte("data1"),
		UseDelta: true,
	})
	require.NoError(t, err)

	ev := waitEvent(t)
	require.False(t, ev.delta, "no previous data means useDelta is false")
	require.Nil(t, ev.prevPub, "no previous state for first publish")
	require.Equal(t, []byte("data1"), ev.pub.Data)

	// Second publish same key — should get prevPub with first data via Redis.
	_, err = broker.Publish(ctx, channel, "key1", centrifuge.MapPublishOptions{
		Data:     []byte("data1_updated"),
		UseDelta: true,
	})
	require.NoError(t, err)

	ev = waitEvent(t)
	require.True(t, ev.delta)
	require.NotNil(t, ev.prevPub)
	require.Equal(t, []byte("data1"), ev.prevPub.Data)
	require.Equal(t, []byte("data1_updated"), ev.pub.Data)

	// Different key — no previous state for this key.
	_, err = broker.Publish(ctx, channel, "key2", centrifuge.MapPublishOptions{
		Data:     []byte("data2"),
		UseDelta: true,
	})
	require.NoError(t, err)

	ev = waitEvent(t)
	require.False(t, ev.delta, "no previous data for key2")
	require.Nil(t, ev.prevPub, "no previous state for key2")

	err = broker.Unsubscribe(channel)
	require.NoError(t, err)
}

// TestPostgresMapBroker_RedisFanout_AdvisoryLockExclusion tests that advisory
// locks ensure only one node per shard polls the stream table.
func TestPostgresMapBroker_RedisFanout_AdvisoryLockExclusion(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	// Create two PG brokers sharing the same advisory lock base ID but
	// with different Redis brokers for fan-out. Both point at the same PG.
	// Only one should hold the lock per shard.

	node1, err := centrifuge.New(centrifuge.Config{
		LogLevel:   centrifuge.LogLevelDebug,
		LogHandler: func(entry centrifuge.LogEntry) {},
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node1.Run())

	node2, err := centrifuge.New(centrifuge.Config{
		LogLevel:   centrifuge.LogLevelDebug,
		LogHandler: func(entry centrifuge.LogEntry) {},
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node2.Run())

	redisBroker1 := newTestRedisBrokerForFanout(t, node1)
	redisBroker2 := newTestRedisBrokerForFanout(t, node2)

	lockBaseID := int64(900000000) + time.Now().UnixNano()%1000000

	var received1, received2 int
	var mu1, mu2 sync.Mutex

	channel := fmt.Sprintf("test_lock_excl_%d", time.Now().UnixNano())

	handler1 := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			if ch == channel {
				mu1.Lock()
				received1++
				mu1.Unlock()
			}
			return nil
		},
	}

	handler2 := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			if ch == channel {
				mu2.Lock()
				received2++
				mu2.Unlock()
			}
			return nil
		},
	}

	numShards := 2

	broker1, err := NewPostgresMapBroker(node1, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  numShards,
		Broker:     redisBroker1,
		Outbox: OutboxConfig{
			PollInterval:          10 * time.Millisecond,
			BatchSize:             100,
			AdvisoryLockBaseID:    lockBaseID,
			AdvisoryLockRetryInterval: 500 * time.Millisecond,
		},
	})
	require.NoError(t, err)
	require.NoError(t, broker1.EnsureSchema(ctx))
	cleanupTestTables(ctx, broker1)

	broker2, err := NewPostgresMapBroker(node2, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  numShards,
		Broker:     redisBroker2,
		Outbox: OutboxConfig{
			PollInterval:          10 * time.Millisecond,
			BatchSize:             100,
			AdvisoryLockBaseID:    lockBaseID,
			AdvisoryLockRetryInterval: 500 * time.Millisecond,
		},
	})
	require.NoError(t, err)

	err = broker1.RegisterEventHandler(handler1)
	require.NoError(t, err)
	err = broker2.RegisterEventHandler(handler2)
	require.NoError(t, err)

	// Subscribe both to the same channel.
	err = broker1.Subscribe(channel)
	require.NoError(t, err)
	err = broker2.Subscribe(channel)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = broker1.Close(context.Background())
		_ = broker2.Close(context.Background())
		_ = node1.Shutdown(context.Background())
		_ = node2.Shutdown(context.Background())
	})

	// Give advisory lock workers time to acquire locks.
	time.Sleep(2 * time.Second)

	// Check advisory locks — for each shard, exactly one session should hold the lock.
	for i := 0; i < numShards; i++ {
		lockID := lockBaseID + int64(i)
		var count int
		err := broker1.pool.QueryRow(ctx,
			"SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND objid = $1 AND granted = true",
			lockID).Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count, "shard %d should have exactly one lock holder", i)
	}

	// Publish some data and verify it arrives via one of the handlers.
	const numMessages = 5
	for i := 0; i < numMessages; i++ {
		_, err = broker1.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte(fmt.Sprintf("data%d", i)),
		})
		require.NoError(t, err)
	}

	// Wait for delivery.
	deadline := time.After(10 * time.Second)
	for {
		mu1.Lock()
		mu2.Lock()
		total := received1 + received2
		mu2.Unlock()
		mu1.Unlock()
		if total >= numMessages {
			break
		}
		select {
		case <-deadline:
			mu1.Lock()
			mu2.Lock()
			t.Fatalf("timeout: received1=%d received2=%d total=%d want=%d",
				received1, received2, received1+received2, numMessages)
			mu2.Unlock()
			mu1.Unlock()
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Both handlers should have received the messages (via Redis PUB/SUB broadcast).
	// But the polling should only happen on one broker per shard.
	mu1.Lock()
	mu2.Lock()
	total := received1 + received2
	mu2.Unlock()
	mu1.Unlock()
	require.GreaterOrEqual(t, total, numMessages,
		"total received should be at least %d", numMessages)
}

// TestPostgresMapBroker_RedisFanout_ClientInfo tests that ClientInfo is preserved
// through Redis PUB/SUB fan-out.
func TestPostgresMapBroker_RedisFanout_ClientInfo(t *testing.T) {
	connString := getPostgresConnString(t)
	ctx := context.Background()

	node, err := centrifuge.New(centrifuge.Config{
		LogLevel:   centrifuge.LogLevelDebug,
		LogHandler: func(entry centrifuge.LogEntry) {},
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Mode: centrifuge.MapModeDurable,
				KeyTTL:        60 * time.Second,
			}
		},
	})
	require.NoError(t, err)
	require.NoError(t, node.Run())

	redisBroker := newTestRedisBrokerForFanout(t, node)

	type pubEvent struct {
		ch  string
		pub *centrifuge.Publication
	}
	eventCh := make(chan pubEvent, 10)

	handler := &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			eventCh <- pubEvent{ch: ch, pub: pub}
			return nil
		},
	}

	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		NumShards:  4,
		Broker:     redisBroker,
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
			BatchSize:    100,
		},
	})
	require.NoError(t, err)
	require.NoError(t, broker.EnsureSchema(ctx))
	cleanupTestTables(ctx, broker)

	err = broker.RegisterEventHandler(handler)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	channel := fmt.Sprintf("test_redis_clientinfo_%d", time.Now().UnixNano())

	err = broker.Subscribe(channel)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	info := &centrifuge.ClientInfo{
		ClientID: "c1",
		UserID:   "u1",
		ConnInfo: []byte("conn"),
		ChanInfo: []byte("chan"),
	}

	_, err = broker.Publish(ctx, channel, "k1", centrifuge.MapPublishOptions{
		Data:       []byte("data1"),
		ClientInfo: info,
	})
	require.NoError(t, err)

	for {
		select {
		case ev := <-eventCh:
			if ev.ch != channel {
				continue
			}
			require.NotNil(t, ev.pub.Info, "ClientInfo should be present in fan-out delivery")
			require.Equal(t, "c1", ev.pub.Info.ClientID)
			require.Equal(t, "u1", ev.pub.Info.UserID)
			require.Equal(t, []byte("conn"), ev.pub.Info.ConnInfo)
			require.Equal(t, []byte("chan"), ev.pub.Info.ChanInfo)

			err = broker.Unsubscribe(channel)
			require.NoError(t, err)
			return
		case <-time.After(15 * time.Second):
			t.Fatal("timeout waiting for publication event")
		}
	}
}
