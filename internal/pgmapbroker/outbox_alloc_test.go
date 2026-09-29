//go:build integration

package pgmapbroker

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// allocBytesPerCall returns the average number of heap bytes fn allocates.
func allocBytesPerCall(runs int, fn func()) uint64 {
	fn() // Warm up connections and caches.
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < runs; i++ {
		fn()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(runs)
}

// Every idle outbox worker polls PollInterval-often (10 times a second by
// default) and usually finds no rows. Such a poll must not allocate buffers
// sized for a full batch: with BatchSize 1000 that is ~150 KB per poll.
func TestPostgresMapBroker_EmptyOutboxBatchDoesNotAllocateBatchBuffers(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	e, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        getPostgresConnString(t),
		NumShards:  1,
		BinaryData: true,
		Outbox: OutboxConfig{
			PollInterval: time.Second,
			BatchSize:    1000,
		},
	})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, e.EnsureSchema(ctx))
	t.Cleanup(func() { _ = e.Close(ctx) })

	// No event handler is registered, so no outbox workers run: only the
	// calls below poll the outbox.
	cursor, err := e.initOutboxCursor(ctx, e.pool)
	require.NoError(t, err)

	buf := &outboxBatchBuf{}
	perCall := allocBytesPerCall(50, func() {
		n, _, err := e.processOutboxBatch(ctx, e.pool, cursor, []int{0}, buf)
		require.NoError(t, err)
		require.Zero(t, n)
	})
	t.Logf("bytes allocated per empty outbox poll: %d", perCall)
	require.Less(t, perCall, uint64(8<<10), "bytes allocated per empty outbox poll")
}
