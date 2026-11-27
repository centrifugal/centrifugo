//go:build integration

package pgmapbroker

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
)

// BenchmarkPostgresMapBroker_EmptyFirstRead reads the state of persistent maps
// nobody read or published to before: each read creates the map's meta.
func BenchmarkPostgresMapBroker_EmptyFirstRead(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()
	var n atomic.Int64
	prefix := fmt.Sprintf("first_%d_", time.Now().UnixNano())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch := prefix + fmt.Sprint(n.Add(1))
			if _, err := broker.ReadState(ctx, ch, centrifuge.MapReadStateOptions{Limit: 100}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.StopTimer()
	var rows int64
	if err := broker.pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE channel LIKE $1`, broker.names.meta), prefix+"%").Scan(&rows); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(rows)/float64(b.N), "rows/op")
}

// BenchmarkPostgresMapBroker_EmptyRepeatRead reads the state of empty maps
// which were read before.
func BenchmarkPostgresMapBroker_EmptyRepeatRead(b *testing.B) {
	broker, cleanup := setupBench(b)
	defer cleanup()
	ctx := context.Background()
	const numChannels = 1000
	prefix := fmt.Sprintf("repeat_%d_", time.Now().UnixNano())
	for i := 0; i < numChannels; i++ {
		if _, err := broker.ReadState(ctx, prefix+fmt.Sprint(i), centrifuge.MapReadStateOptions{Limit: 100}); err != nil {
			b.Fatal(err)
		}
	}
	var n atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch := prefix + fmt.Sprint(n.Add(1)%numChannels)
			if _, err := broker.ReadState(ctx, ch, centrifuge.MapReadStateOptions{Limit: 100}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
