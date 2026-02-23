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

func setupPostgresMapBrokerBench(b *testing.B) (*PostgresMapBroker, func()) {
	b.Helper()

	connString := getPostgresConnString(b)

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				SyncMode:      centrifuge.MapSyncConverging,
				RetentionMode: centrifuge.MapRetentionPermanent,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
	})
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	if err := broker.EnsureSchema(ctx); err != nil {
		b.Fatal(err)
	}
	_ = broker.RegisterEventHandler(nil)

	// Clean up tables
	cleanupTestTables(ctx, broker)

	return broker, func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	}
}

func setupPostgresMapBrokerBenchOrdered(b *testing.B) (*PostgresMapBroker, func()) {
	b.Helper()

	connString := getPostgresConnString(b)

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				Ordered:       true,
				SyncMode:      centrifuge.MapSyncConverging,
				RetentionMode: centrifuge.MapRetentionExpiring,
				KeyTTL:        time.Minute,
			}
		},
	})
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
	})
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	if err := broker.EnsureSchema(ctx); err != nil {
		b.Fatal(err)
	}
	_ = broker.RegisterEventHandler(nil)

	// Clean up tables
	cleanupTestTables(ctx, broker)

	return broker, func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	}
}

// BenchmarkPostgresMapBroker_PublishStreamOnly benchmarks publishing to stream.
func BenchmarkPostgresMapBroker_PublishStreamOnly(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_stream_%d", i%1000)
			data := []byte(fmt.Sprintf("message_%d", i))
			_, err := broker.Publish(ctx, ch, "", centrifuge.MapPublishOptions{
				Data: data,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_PublishMapStateSimple benchmarks simple keyed state.
func BenchmarkPostgresMapBroker_PublishMapStateSimple(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_map_simple_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			data := []byte(fmt.Sprintf("data%d", i))
			_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data: data,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_PublishMapStateOrdered benchmarks ordered keyed state.
func BenchmarkPostgresMapBroker_PublishMapStateOrdered(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBenchOrdered(b)
	defer cleanup()

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_map_ordered_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			data := []byte(fmt.Sprintf("data%d", i))
			_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data:  data,
				Score: i,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_PublishCombined benchmarks publishing with stream + state.
func BenchmarkPostgresMapBroker_PublishCombined(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_combined_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			data := []byte(fmt.Sprintf("data%d", i))
			_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data: data,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_ReadStream benchmarks reading from stream.
func BenchmarkPostgresMapBroker_ReadStream(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()
	channel := "bench_read_stream"

	// Prepopulate stream with 1000 messages
	var sp centrifuge.StreamPosition
	for i := 0; i < 1000; i++ {
		data := []byte(fmt.Sprintf("message_%d", i))
		res, err := broker.Publish(ctx, channel, "", centrifuge.MapPublishOptions{
			Data: data,
		})
		if err != nil {
			b.Fatal(err)
		}
		sp = res.Position
	}

	b.ReportAllocs()
	b.ResetTimer()

	sp.Offset = sp.Offset - 1000

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := broker.ReadStream(ctx, channel, centrifuge.MapReadStreamOptions{
				Filter: centrifuge.StreamFilter{
					Limit: 1000,
					Since: &sp,
				},
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_ReadStateFull benchmarks reading full unordered state.
func BenchmarkPostgresMapBroker_ReadStateFull(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()
	channel := "bench_read_state"

	// Prepopulate state with 1000 entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		data := []byte(fmt.Sprintf("data%d", i))
		_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
			Data: data,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
				Limit: -1, // Read all
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_ReadStatePaginated benchmarks paginated state reads.
func BenchmarkPostgresMapBroker_ReadStatePaginated(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()
	channel := "bench_read_state_paginated"

	// Prepopulate state with 1000 entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		data := []byte(fmt.Sprintf("data%d", i))
		_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
			Data: data,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
				Limit: 100, // Read 100 at a time
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_ReadStateOrdered benchmarks reading ordered state.
func BenchmarkPostgresMapBroker_ReadStateOrdered(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBenchOrdered(b)
	defer cleanup()

	ctx := context.Background()
	channel := "bench_read_state_ordered"

	// Prepopulate ordered state with 1000 entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		data := []byte(fmt.Sprintf("data%d", i))
		_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
			Data:  data,
			Score: int64(i),
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
				Limit: 100,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_Stats benchmarks reading state statistics.
func BenchmarkPostgresMapBroker_Stats(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()
	channel := "bench_stats"

	// Prepopulate state with 1000 entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		data := []byte(fmt.Sprintf("data%d", i))
		_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
			Data: data,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := broker.Stats(ctx, channel)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_Remove benchmarks removing keys.
func BenchmarkPostgresMapBroker_Remove(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()

	// Prepopulate 100 channels × 100 keys.
	for ch := 0; ch < 100; ch++ {
		channel := fmt.Sprintf("bench_remove_%d", ch)
		for k := 0; k < 100; k++ {
			key := fmt.Sprintf("key%d", k)
			_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
				Data: []byte("data"),
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_remove_%d", i%100)
			key := fmt.Sprintf("key%d", i%100)
			_, _ = broker.Remove(ctx, ch, key, centrifuge.MapRemoveOptions{})
		}
	})
}

// BenchmarkPostgresMapBroker_IdempotentPublish benchmarks idempotent publishing.
func BenchmarkPostgresMapBroker_IdempotentPublish(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_idempotent_%d", i%1000)
			data := []byte(fmt.Sprintf("message_%d", i))
			idempotencyKey := fmt.Sprintf("key_%d", i)
			_, err := broker.Publish(ctx, ch, "", centrifuge.MapPublishOptions{
				Data:                data,
				IdempotencyKey:      idempotencyKey,
				IdempotentResultTTL: 60 * time.Second,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_CAS benchmarks CAS (Compare-And-Swap) operations.
func BenchmarkPostgresMapBroker_CAS(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()
	channel := "bench_cas"

	// Create initial key
	res, err := broker.Publish(ctx, channel, "shared_counter", centrifuge.MapPublishOptions{
		Data: []byte("0"),
	})
	if err != nil {
		b.Fatal(err)
	}
	lastPos := res.Position

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Read current state
			stateRes, err := broker.ReadState(ctx, channel, centrifuge.MapReadStateOptions{
				Key: "shared_counter",
			})
			entries, pos, _ := stateRes.Publications, stateRes.Position, stateRes.Cursor
			if err != nil || len(entries) == 0 {
				continue
			}

			// Attempt CAS
			expectedPos := centrifuge.StreamPosition{Offset: entries[0].Offset, Epoch: pos.Epoch}
			_, _ = broker.Publish(ctx, channel, "shared_counter", centrifuge.MapPublishOptions{
				Data:             []byte("updated"),
				ExpectedPosition: &expectedPos,
			})
		}
	})
	_ = lastPos
}

// ============================================================================
// Outbox Mode Benchmarks
// ============================================================================

func setupPostgresMapBrokerOutboxBench(b *testing.B) (*PostgresMapBroker, func()) {
	b.Helper()

	connString := getPostgresConnString(b)

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				SyncMode:      centrifuge.MapSyncConverging,
				RetentionMode: centrifuge.MapRetentionPermanent,
			}
		},
	})
	// Use fewer shards than pool size to leave connections available for Publish calls.
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		PoolSize:   32,
		NumShards:  8, // Must be less than PoolSize to leave room for Publish
		Outbox: OutboxConfig{
			PollInterval: 10 * time.Millisecond,
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
	_ = broker.RegisterEventHandler(nil)

	// Clean up tables
	cleanupTestTables(ctx, broker)

	return broker, func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	}
}

// setupPostgresMapBrokerOutboxBenchWithHandler creates broker and registers handler.
func setupPostgresMapBrokerOutboxBenchWithHandler(b *testing.B, handler centrifuge.BrokerEventHandler) (*PostgresMapBroker, func()) {
	b.Helper()

	connString := getPostgresConnString(b)

	node, _ := centrifuge.New(centrifuge.Config{
		GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
			return centrifuge.MapChannelOptions{
				SyncMode:      centrifuge.MapSyncConverging,
				RetentionMode: centrifuge.MapRetentionPermanent,
			}
		},
	})
	// Use fewer shards than pool size to leave connections available for Publish calls.
	broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
		DSN:        connString,
		BinaryData: true,
		PoolSize:   32,
		NumShards:  8, // Must be less than PoolSize to leave room for Publish
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

	// Clean up tables before registering handler (which starts workers)
	cleanupTestTables(ctx, broker)

	// Register handler which starts workers
	_ = broker.RegisterEventHandler(handler)

	return broker, func() {
		_ = broker.Close(context.Background())
		_ = node.Shutdown(context.Background())
	}
}

// BenchmarkPostgresMapBroker_OutboxPublish benchmarks publishing with outbox mode.
func BenchmarkPostgresMapBroker_OutboxPublish(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerOutboxBench(b)
	defer cleanup()

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			ch := fmt.Sprintf("bench_outbox_publish_%d", i%1000)
			key := fmt.Sprintf("key%d", i%1000)
			data := []byte(fmt.Sprintf("data%d", i))
			_, err := broker.Publish(ctx, ch, key, centrifuge.MapPublishOptions{
				Data: data,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresMapBroker_OutboxThroughput measures outbox delivery throughput
// with parallel publishing across multiple channels (utilizes all shard workers).
func BenchmarkPostgresMapBroker_OutboxThroughput(b *testing.B) {
	ctx := context.Background()
	prefix := fmt.Sprintf("bench_outbox_%d_", time.Now().UnixNano())

	// Track deliveries
	var delivered int64
	doneCh := make(chan struct{})
	var mu sync.Mutex
	perChannelOffsets := make(map[string]uint64)

	broker, cleanup := setupPostgresMapBrokerOutboxBenchWithHandler(b, &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			mu.Lock()
			o, ok := perChannelOffsets[ch]
			if ok && sp.Offset != o+1 {
				b.Logf("offset %d != %d", sp.Offset, o+1)
			}
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

	// Give outbox workers a moment to start polling.
	time.Sleep(100 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	// Publish messages in parallel across multiple channels.
	var counter int64
	var wg sync.WaitGroup
	numPublishers := 8
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

	// Wait for all deliveries
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		b.Fatalf("timeout: only delivered %d/%d", atomic.LoadInt64(&delivered), b.N)
	}

	b.StopTimer()
}

// BenchmarkPostgresMapBroker_PublishHighVolume benchmarks publish into a channel with a large stream.
func BenchmarkPostgresMapBroker_PublishHighVolume(b *testing.B) {
	broker, cleanup := setupPostgresMapBrokerBench(b)
	defer cleanup()

	ctx := context.Background()
	channel := fmt.Sprintf("bench_highvol_%d", time.Now().UnixNano())

	// Pre-fill stream with 10000 entries.
	for i := 0; i < 10000; i++ {
		_, err := broker.Publish(ctx, channel, fmt.Sprintf("prefill_%d", i), centrifuge.MapPublishOptions{
			Data: []byte("data"),
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%d", i%1000)
		_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
			Data: []byte("x"),
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPostgresMapBroker_OutboxLatency measures publish-to-delivery latency.
func BenchmarkPostgresMapBroker_OutboxLatency(b *testing.B) {
	ctx := context.Background()
	channel := fmt.Sprintf("bench_outbox_latency_%d", time.Now().UnixNano())

	latencies := make(chan time.Duration, b.N)
	publishTimes := make(map[string]time.Time)
	var mu sync.Mutex

	broker, cleanup := setupPostgresMapBrokerOutboxBenchWithHandler(b, &testBrokerEventHandler{
		HandlePublicationFunc: func(ch string, pub *centrifuge.Publication, sp centrifuge.StreamPosition, delta bool, prevPub *centrifuge.Publication) error {
			if ch != channel {
				return nil
			}
			mu.Lock()
			if publishTime, ok := publishTimes[pub.Key]; ok {
				latency := time.Since(publishTime)
				select {
				case latencies <- latency:
				default:
				}
				delete(publishTimes, pub.Key)
			}
			mu.Unlock()
			return nil
		},
	})
	defer cleanup()

	// Give outbox worker a moment to start polling.
	time.Sleep(100 * time.Millisecond)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key%d", i)
		data := []byte(fmt.Sprintf("data%d", i))

		mu.Lock()
		publishTimes[key] = time.Now()
		mu.Unlock()

		_, err := broker.Publish(ctx, channel, key, centrifuge.MapPublishOptions{
			Data: data,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	// Collect latencies
	timeout := time.After(60 * time.Second)
	var totalLatency time.Duration
	var count int

loop:
	for {
		select {
		case l := <-latencies:
			totalLatency += l
			count++
			if count >= b.N {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	b.StopTimer()

	if count > 0 {
		avgLatency := totalLatency / time.Duration(count)
		b.ReportMetric(float64(avgLatency.Microseconds()), "us/op")
	}
}

// BenchmarkPostgresMapBroker_Cleanup benchmarks TTL-based key cleanup throughput.
// Measures how fast the expire_keys SQL function processes expired keys.
// Throughput in keys/second = keys/op * 1e9 / ns_per_op.
func BenchmarkPostgresMapBroker_Cleanup(b *testing.B) {
	for _, ordered := range []bool{false, true} {
		orderLabel := "unordered"
		if ordered {
			orderLabel = "ordered"
		}
		for _, numKeys := range []int{100, 1000, 10000} {
			b.Run(fmt.Sprintf("%s/keys_%d", orderLabel, numKeys), func(b *testing.B) {
				connString := getPostgresConnString(b)
				node, _ := centrifuge.New(centrifuge.Config{
					GetMapChannelOptions: func(channel string) centrifuge.MapChannelOptions {
						return centrifuge.MapChannelOptions{
							SyncMode:      centrifuge.MapSyncConverging,
							RetentionMode: centrifuge.MapRetentionExpiring,
							KeyTTL:        time.Millisecond,
							Ordered:       ordered,
						}
					},
				})
				broker, err := NewPostgresMapBroker(node, PostgresMapBrokerConfig{
					DSN:              connString,
					BinaryData:       true,
					TTLCheckInterval: time.Hour,  // Prevent background TTL worker interference.
					CleanupInterval:  time.Hour,  // Prevent background cleanup worker interference.
				})
				if err != nil {
					b.Fatal(err)
				}
				ctx := context.Background()
				if err := broker.EnsureSchema(ctx); err != nil {
					b.Fatal(err)
				}
				_ = broker.RegisterEventHandler(&testBrokerEventHandler{})
				cleanupTestTables(ctx, broker)
				b.Cleanup(func() {
					_ = broker.Close(context.Background())
					_ = node.Shutdown(context.Background())
				})

				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					ch := fmt.Sprintf("test_cleanup_%d_%d", time.Now().UnixNano(), i)
					for k := 0; k < numKeys; k++ {
						_, err := broker.Publish(ctx, ch, fmt.Sprintf("key%d", k), centrifuge.MapPublishOptions{
							Data:  []byte("data"),
							Score: int64(k),
						})
						if err != nil {
							b.Fatal(err)
						}
					}
					time.Sleep(2 * time.Millisecond)
					b.StartTimer()

					// expireKeys processes up to 1000 keys per channel per call.
					calls := (numKeys + 999) / 1000
					for c := 0; c <= calls; c++ {
						broker.expireKeys(ctx)
					}
				}
				b.ReportMetric(float64(numKeys), "keys/op")
			})
		}
	}
}
