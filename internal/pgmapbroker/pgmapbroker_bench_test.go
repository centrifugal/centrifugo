//go:build integration

package pgmapbroker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
)

// setupBench creates a broker with Persistent mode for standard benchmarks.
func setupBench(b *testing.B) (*PostgresMapBroker, func()) {
	b.Helper()
	connString := getPostgresConnString(b)
	node, _ := centrifuge.New(centrifuge.Config{
		Map: centrifuge.MapConfig{
			GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
				return centrifuge.MapChannelOptions{
					Mode: centrifuge.MapModePersistent,
				}
			},
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:                    connString,
		BinaryData:             true,
		PartitionRetentionDays: 7,
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	if err := broker.EnsureSchema(ctx); err != nil {
		b.Fatal(err)
	}
	_ = broker.RegisterEventHandler(nil)
	cleanupTestTables(ctx, broker)
	return broker, func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	}
}

// setupBenchOrdered creates a broker with Recoverable+Ordered mode.
func setupBenchOrdered(b *testing.B) (*PostgresMapBroker, func()) {
	b.Helper()
	connString := getPostgresConnString(b)
	node, _ := centrifuge.New(centrifuge.Config{
		Map: centrifuge.MapConfig{
			GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
				return centrifuge.MapChannelOptions{
					Ordered: true,
					Mode:    centrifuge.MapModeRecoverable,
					KeyTTL:  time.Minute,
				}
			},
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:                    connString,
		BinaryData:             true,
		PartitionRetentionDays: 7,
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	if err := broker.EnsureSchema(ctx); err != nil {
		b.Fatal(err)
	}
	_ = broker.RegisterEventHandler(nil)
	cleanupTestTables(ctx, broker)
	return broker, func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	}
}

// setupBenchOutbox creates a broker with outbox workers running.
func setupBenchOutbox(b *testing.B, handler centrifuge.BrokerEventHandler) (*PostgresMapBroker, func()) {
	b.Helper()
	connString := getPostgresConnString(b)
	node, _ := centrifuge.New(centrifuge.Config{
		Map: centrifuge.MapConfig{
			GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
				return centrifuge.MapChannelOptions{
					Mode: centrifuge.MapModePersistent,
				}
			},
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:                    connString,
		BinaryData:             true,
		PoolSize:               32,
		NumShards:              8,
		PartitionRetentionDays: 7,
		Outbox: OutboxConfig{
			PollInterval: 5 * time.Millisecond,
			BatchSize:    1000,
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	if err := broker.EnsureSchema(ctx); err != nil {
		b.Fatal(err)
	}
	cleanupTestTables(ctx, broker)
	_ = broker.RegisterEventHandler(handler)
	return broker, func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	}
}

// ============================================================================
// Publish benchmarks
// ============================================================================

// BenchmarkPostgresMapBroker_Publish measures the core map publish path:
// shard lock → meta UPSERT → state UPSERT → stream INSERT → NOTIFY.
// All publish benchmarks go through the same cf_map_publish SQL function.
func BenchmarkPostgresMapBroker_Publish(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_publish_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data: []byte("x"),
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_PublishOrdered measures publish with score-based
// ordering (sorted set maintenance in the state table).
func BenchmarkPostgresMapBroker_PublishOrdered(b *testing.B) {
	broker, cleanup := setupBenchOrdered(b)
	defer cleanup()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_ordered_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data:  []byte("x"),
				Score: i,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_PublishIdempotent measures publish with
// idempotency key check (extra lookup + conditional insert).
func BenchmarkPostgresMapBroker_PublishIdempotent(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_idempotent_%d", i%1000)
			key := fmt.Sprintf("key_%d", i)
			_, err := broker.Publish(ctx, ch, "", centrifuge.MapPublishOptions{
				Data:                []byte(key),
				IdempotencyKey:      key,
				IdempotentResultTTL: 60 * time.Second,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_PublishCAS measures Compare-And-Swap: read
// current state + conditional publish. Includes both the read and write
// round-trips.
func BenchmarkPostgresMapBroker_PublishCAS(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()
	channel := "bench_cas"

	// Seed the key.
	_, err := broker.Publish(ctx, channel, "counter", centrifuge.MapPublishOptions{
		Data: []byte("0"),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
				Key: "counter",
			})
			if err != nil || len(stateRes.Publications) == 0 {
				continue
			}
			pos := centrifuge.StreamPosition{
				Offset: stateRes.Publications[0].Offset,
				Epoch:  stateRes.Position.Epoch,
			}
			_, _ = broker.Publish(ctx, channel, "counter", centrifuge.MapPublishOptions{
				Data:             []byte("updated"),
				ExpectedPosition: &pos,
			})
		}
	})
}

// ============================================================================
// Read benchmarks
// ============================================================================

// BenchmarkPostgresMapBroker_ReadStream measures reading publications from
// the stream table (the change log used for recovery).
func BenchmarkPostgresMapBroker_ReadStream(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()
	channel := "bench_read_stream"

	// Seed 100 publications.
	for i := 0; i < 100; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("k%d", i), centrifuge.MapPublishOptions{
			Data: []byte("data"),
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
			Filter: centrifuge.StreamFilter{Limit: 50},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPostgresMapBroker_ReadState measures reading the full key-value
// snapshot of a channel.
func BenchmarkPostgresMapBroker_ReadState(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()
	channel := "bench_read_state"

	for i := 0; i < 100; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("k%d", i), centrifuge.MapPublishOptions{
			Data: []byte("data"),
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
			Limit: 100,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPostgresMapBroker_ReadStatePaginated measures paginated state
// reads (typical for large channels with cursor-based iteration).
func BenchmarkPostgresMapBroker_ReadStatePaginated(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()
	channel := "bench_read_state_paged"

	for i := 0; i < 100; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("k%d", i), centrifuge.MapPublishOptions{
			Data: []byte("data"),
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
			Limit: 10,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPostgresMapBroker_ReadStateOrdered measures reading ordered state
// (sorted by score — the leaderboard/ranking access pattern).
func BenchmarkPostgresMapBroker_ReadStateOrdered(b *testing.B) {
	broker, cleanup := setupBenchOrdered(b)
	defer cleanup()
	ctx := context.Background()
	channel := "bench_read_state_ordered"

	for i := 0; i < 100; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("k%d", i), centrifuge.MapPublishOptions{
			Data:  []byte("data"),
			Score: int64(i),
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
			Limit: 50,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPostgresMapBroker_Stats measures the channel statistics query.
func BenchmarkPostgresMapBroker_Stats(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()
	channel := "bench_stats"

	for i := 0; i < 50; i++ {
		_, _ = broker.Publish(ctx, channel, fmt.Sprintf("k%d", i), centrifuge.MapPublishOptions{
			Data: []byte("data"),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := broker.Stats(ctx, channel)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPostgresMapBroker_Remove measures key removal.
func BenchmarkPostgresMapBroker_Remove(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_remove_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			// Ensure key exists, then remove it.
			_, _ = broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data: []byte("x"),
			})
			_, err := broker.Remove(ctx, ch, key, centrifuge.MapRemoveOptions{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// ============================================================================
// Outbox delivery benchmarks
// ============================================================================

// BenchmarkPostgresMapBroker_OutboxPublish measures publish latency with
// the outbox worker running (parallel publishers across many channels).
func BenchmarkPostgresMapBroker_OutboxPublish(b *testing.B) {
	broker, cleanup := setupBenchOutbox(b, nil)
	defer cleanup()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_outbox_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data: []byte("x"),
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_OutboxThroughput measures end-to-end outbox
// throughput: 8 publisher goroutines + outbox workers delivering to a handler.
// Verifies per-channel offset ordering as a side effect.
func BenchmarkPostgresMapBroker_OutboxThroughput(b *testing.B) {
	ctx := context.Background()
	prefix := fmt.Sprintf("bench_tp_%d_", time.Now().UnixNano())

	var delivered int64
	doneCh := make(chan struct{})
	var mu sync.Mutex
	perChannelOffsets := make(map[string]uint64)

	broker, cleanup := setupBenchOutbox(b, &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			mu.Lock()
			perChannelOffsets[ch] = sp.Offset
			mu.Unlock()
			if atomic.AddInt64(&delivered, 1) >= int64(b.N) {
				select {
				case <-doneCh:
				default:
					close(doneCh)
				}
			}
			return nil
		},
	})
	defer cleanup()
	time.Sleep(100 * time.Millisecond) // let workers start

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	var wg sync.WaitGroup
	const numPublishers = 8
	perPublisher := b.N / numPublishers
	remainder := b.N % numPublishers
	for p := 0; p < numPublishers; p++ {
		n := perPublisher
		if p < remainder {
			n++
		}
		if n == 0 {
			continue
		}
		wg.Add(1)
		go func(count int) {
			defer wg.Done()
			for j := 0; j < count; j++ {
				i := atomic.AddInt64(&counter, 1)
				ch := fmt.Sprintf("%sch%d", prefix, i%100)
				key := fmt.Sprintf("key%d", i)
				_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
					Data: []byte("x"),
				})
				if err != nil {
					b.Error(err)
					return
				}
			}
		}(n)
	}
	wg.Wait()

	select {
	case <-doneCh:
	case <-time.After(30 * time.Second):
		b.Fatalf("timeout: delivered %d/%d", atomic.LoadInt64(&delivered), b.N)
	}
	b.StopTimer()
}

// BenchmarkPostgresMapBroker_OutboxLatency measures publish-to-delivery
// latency via the outbox. Reports average latency as a custom metric.
func BenchmarkPostgresMapBroker_OutboxLatency(b *testing.B) {
	ctx := context.Background()
	channel := fmt.Sprintf("bench_latency_%d", time.Now().UnixNano())

	publishTimes := make([]time.Time, b.N+2)
	delivered := make(chan time.Duration, b.N)
	var mu sync.Mutex

	broker, cleanup := setupBenchOutbox(b, &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			if ch != channel {
				return nil
			}
			mu.Lock()
			var t time.Time
			if int(sp.Offset) < len(publishTimes) {
				t = publishTimes[sp.Offset]
			}
			mu.Unlock()
			if !t.IsZero() {
				delivered <- time.Since(t)
			}
			return nil
		},
	})
	defer cleanup()
	time.Sleep(100 * time.Millisecond) // let workers start

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mu.Lock()
		now := time.Now()
		res, err := broker.Publish(ctx, channel, fmt.Sprintf("key%d", i), centrifuge.MapPublishOptions{
			Data: []byte("x"),
		})
		if err != nil {
			mu.Unlock()
			b.Fatal(err)
		}
		if int(res.Position.Offset) < len(publishTimes) {
			publishTimes[res.Position.Offset] = now
		}
		mu.Unlock()
	}

	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	var totalLatency time.Duration
	received := 0
	for received < b.N {
		select {
		case d := <-delivered:
			totalLatency += d
			received++
		case <-deadline.C:
			b.Fatalf("timeout: received %d/%d", received, b.N)
		}
	}
	b.StopTimer()

	if b.N > 0 {
		avg := totalLatency / time.Duration(b.N)
		b.ReportMetric(float64(avg.Microseconds()), "µs/delivery")
	}
}

// ============================================================================
// Cleanup benchmark
// ============================================================================

// BenchmarkPostgresMapBroker_Cleanup measures TTL-based key expiration
// throughput via the expire_keys SQL function. Sub-benchmarks cover
// different key counts and ordered/unordered modes.
func BenchmarkPostgresMapBroker_Cleanup(b *testing.B) {
	for _, ordered := range []bool{false, true} {
		orderLabel := "unordered"
		if ordered {
			orderLabel = "ordered"
		}
		for _, numKeys := range []int{100, 1000} {
			b.Run(fmt.Sprintf("%s/keys_%d", orderLabel, numKeys), func(b *testing.B) {
				connString := getPostgresConnString(b)
				node, _ := centrifuge.New(centrifuge.Config{
					Map: centrifuge.MapConfig{
						GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
							return centrifuge.MapChannelOptions{
								Mode:    centrifuge.MapModeRecoverable,
								KeyTTL:  time.Millisecond,
								Ordered: ordered,
							}
						},
					},
				})
				broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
					DSN:                    connString,
					BinaryData:             true,
					PartitionRetentionDays: 7,
				})
				if err != nil {
					b.Fatal(err)
				}
				ctx := context.Background()
				if err := broker.EnsureSchema(ctx); err != nil {
					b.Fatal(err)
				}
				_ = broker.RegisterEventHandler(nil)
				defer func() {
					_ = broker.Close(context.Background())
					_ = node.Shutdown(context.Background())
				}()

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					channel := fmt.Sprintf("bench_cleanup_%d_%d", time.Now().UnixNano(), i)
					// Insert keys with already-expired TTL.
					for k := 0; k < numKeys; k++ {
						_, err := broker.Publish(ctx, channel, fmt.Sprintf("key_%d", k), centrifuge.MapPublishOptions{
							Data: []byte("data"),
						})
						if err != nil {
							b.Fatal(err)
						}
					}
					// Wait for TTL to expire.
					time.Sleep(5 * time.Millisecond)
					b.StartTimer()

					// Run expire_keys.
					numShards := broker.conf.NumShards
					var metaTTL *string
					_, err := broker.pool.Exec(ctx, fmt.Sprintf(`
						SELECT %s($1, $2, $3::interval, $4)
					`, broker.names.expireKeys), 1000, numShards, metaTTL, channel)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(numKeys), "keys/op")
			})
		}
	}
}
