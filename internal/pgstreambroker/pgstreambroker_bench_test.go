//go:build integration

package pgstreambroker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// newBenchPostgresStreamBroker is a slim broker constructor for benchmarks.
// It uses BinaryData=true (so we don't pay JSON validation cost) and a
// large outbox batch size for throughput tests.
func newBenchPostgresStreamBroker(b *testing.B) (*PostgresStreamBroker, *centrifuge.Node, *testBrokerEventHandler) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(b, err)

	connString := getPostgresConnString(b)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                    connString,
		BinaryData:             true,
		CleanupInterval:        1 * time.Minute, // long — don't interfere with bench
		StreamRetention:        24 * time.Hour,
		PartitionLookaheadDays: 1,
		PartitionRetentionDays: 7,
		Outbox: OutboxConfig{
			BatchSize: 1000,
			// PollInterval uses default (50ms) — matches map broker bench
			// config so numbers are comparable. Lower poll intervals steal
			// PG capacity from the publish goroutines.
		},
	})
	require.NoError(b, err)

	ctx := context.Background()
	hardResetTestSchema(b, e)
	require.NoError(b, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	handler := &testBrokerEventHandler{}
	require.NoError(b, e.RegisterBrokerEventHandler(handler))
	require.NoError(b, node.Run())

	b.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})
	return e, node, handler
}

// BenchmarkPostgresStreamBroker_Publish measures the core publish path with
// parallel goroutines across many channels (same structure as the map broker
// Publish benchmark). This is the steady-state number to report.
func BenchmarkPostgresStreamBroker_Publish(b *testing.B) {
	e, _, _ := newBenchPostgresStreamBroker(b)

	data := []byte("hello world")

	b.ResetTimer()
	b.ReportAllocs()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_publish_%d", i%1000)
			_, err := e.Publish(ch, data, centrifuge.PublishOptions{
				HistoryTTL:  10 * time.Minute,
				HistorySize: 1000,
			})
			if err != nil {
				b.Fatalf("publish: %v", err)
			}
		}
	})
}

// BenchmarkPostgresStreamBroker_OutboxLatency measures publish→delivery
// end-to-end latency: time from Publish() return to HandlePublication() call
// in the outbox worker. Reports the average latency via b.ReportMetric.
func BenchmarkPostgresStreamBroker_OutboxLatency(b *testing.B) {
	e, _, handler := newBenchPostgresStreamBroker(b)

	channel := "bench_latency"
	data := []byte("hello world")

	// Pre-allocate a slice indexed by offset (1-based). The publish loop
	// stores the publish time at position res.Offset, then unlocks. The
	// handler reads at pub.Offset. Mutex serializes the store/load against
	// the publish call so the handler always sees the time before the
	// outbox worker delivers the row.
	publishTimes := make([]time.Time, b.N+2)
	var mu sync.Mutex
	delivered := make(chan time.Duration, b.N)

	handler.HandlePublicationFunc = func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
		if ch != channel {
			return nil
		}
		mu.Lock()
		var t time.Time
		if int(pub.Offset) < len(publishTimes) {
			t = publishTimes[pub.Offset]
		}
		mu.Unlock()
		if !t.IsZero() {
			delivered <- time.Since(t)
		}
		return nil
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Hold the lock around publish + array store so the handler sees
		// the publish time before processing the outbox row.
		mu.Lock()
		now := time.Now()
		res, err := e.Publish(channel, data, centrifuge.PublishOptions{
			HistoryTTL:  10 * time.Minute,
			HistorySize: 10000,
		})
		if err != nil {
			mu.Unlock()
			b.Fatalf("publish: %v", err)
		}
		if int(res.Offset) < len(publishTimes) {
			publishTimes[res.Offset] = now
		}
		mu.Unlock()
	}

	// Wait for all deliveries.
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	received := 0
	totalLatency := time.Duration(0)
	for received < b.N {
		select {
		case d := <-delivered:
			totalLatency += d
			received++
		case <-deadline.C:
			b.Fatalf("only received %d/%d deliveries within 30s", received, b.N)
		}
	}

	b.StopTimer()
	if b.N > 0 {
		avg := totalLatency / time.Duration(b.N)
		b.ReportMetric(float64(avg.Microseconds()), "µs/delivery")
	}
}

// BenchmarkPostgresStreamBroker_RetentionDrop measures partition drop time.
// Pre-populates b.N partitions each with rowsPerPartition rows, then times
// only the DROP TABLE calls (the create + populate happens with the timer
// stopped).
func BenchmarkPostgresStreamBroker_RetentionDrop(b *testing.B) {
	e, _, _ := newBenchPostgresStreamBroker(b)
	ctx := context.Background()

	const rowsPerPartition = 10000
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	partNames := make([]string, b.N)

	// Stop the timer for the setup phase.
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		partName := fmt.Sprintf("%s_bench_%d_%d", e.names.history, time.Now().UnixNano(), i)
		// Use date ranges many years in the past to avoid collisions with
		// the lookahead-managed partitions.
		yearsAgo := yesterday.AddDate(-100-i, 0, 0)
		yearsAgoNext := yearsAgo.AddDate(0, 0, 1)
		_, err := e.pool.Exec(ctx, fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
			partName, e.names.history,
			yearsAgo.Format("2006-01-02"), yearsAgoNext.Format("2006-01-02"),
		))
		require.NoError(b, err)
		partNames[i] = partName

		// Pre-populate the partition.
		for r := 0; r < rowsPerPartition; r++ {
			_, err := e.pool.Exec(ctx, fmt.Sprintf(
				`INSERT INTO %s (channel, channel_offset, kind, data, created_at, shard_id) VALUES ($1, $2, 0, '\x'::bytea, $3, 0)`,
				e.names.history,
			), fmt.Sprintf("bench_drop_%d_%d", i, r), int64(r), yearsAgo.Add(time.Duration(r)*time.Microsecond))
			require.NoError(b, err)
		}
	}

	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.pool.Exec(ctx, fmt.Sprintf("DROP TABLE %s", partNames[i]))
		require.NoError(b, err)
	}
}

// BenchmarkPostgresStreamBroker_FineGrainedDelete measures the chunked DELETE
// throughput of the FineGrainedHistoryCleanup pass.
func BenchmarkPostgresStreamBroker_FineGrainedDelete(b *testing.B) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(b, err)

	connString := getPostgresConnString(b)
	e, err := NewPostgresStreamBroker(node, PostgresStreamBrokerConfig{
		DSN:                       connString,
		NumShards:                 16,
		BinaryData:                true,
		CleanupInterval:           24 * time.Hour, // disable auto-cleanup
		StreamRetention:           24 * time.Hour,
		PartitionLookaheadDays:    1,
		PartitionRetentionDays:    7,
		FineGrainedHistoryCleanup: true,
		CleanupBatchSize:          1000,
		CleanupChunkPause:         1 * time.Millisecond,
		Outbox: OutboxConfig{
			PollInterval: 5 * time.Millisecond,
			BatchSize:    1000,
		},
	})
	require.NoError(b, err)

	ctx := context.Background()
	hardResetTestSchema(b, e)
	require.NoError(b, e.EnsureSchema(ctx))
	cleanupTestTables(ctx, e)

	require.NoError(b, e.RegisterBrokerEventHandler(&testBrokerEventHandler{}))
	require.NoError(b, node.Run())
	b.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = node.Shutdown(context.Background())
	})

	const rowsPerIteration = 5000
	channel := "bench_finegrained"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Pre-populate with rows that have an already-expired history_ttl.
		_, err := e.pool.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s (channel, top_offset, epoch, history_ttl, history_size, meta_expires_at, updated_at)
			 VALUES ($1, 0, 'e', '1 millisecond', 10000, NOW() + '1 hour', NOW())
			 ON CONFLICT (channel) DO UPDATE SET history_ttl = EXCLUDED.history_ttl`,
			e.names.meta,
		), channel)
		require.NoError(b, err)

		for r := 0; r < rowsPerIteration; r++ {
			_, err := e.pool.Exec(ctx, fmt.Sprintf(
				`INSERT INTO %s (channel, channel_offset, kind, data, created_at, shard_id)
				 VALUES ($1, $2, 0, '\x'::bytea, NOW() - INTERVAL '1 hour', 0)`,
				e.names.history,
			), channel, int64(r))
			require.NoError(b, err)
		}

		b.StartTimer()
		e.cleanupHistoryFineGrained(ctx)
		b.StopTimer()
	}
}
