//go:build integration

package pgstreambroker

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// benchReadOpts reads the position only, as subscribe and position checks do.
var benchReadOpts = centrifuge.HistoryOptions{}

// metaRows counts the meta rows of channels starting with prefix.
func metaRows(b *testing.B, e *PostgresStreamBroker, prefix string) float64 {
	var n int64
	require.NoError(b, e.pool.QueryRow(context.Background(), fmt.Sprintf(`SELECT count(*) FROM %s WHERE channel LIKE $1`, e.names.meta), prefix+"%").Scan(&n))
	return float64(n)
}

// BenchmarkPostgresStreamBroker_EmptyFirstRead reads the position of channels
// nobody read or published to before: each read creates the channel's meta.
func BenchmarkPostgresStreamBroker_EmptyFirstRead(b *testing.B) {
	e, _, _ := newBenchPostgresStreamBroker(b)
	var n atomic.Int64
	prefix := fmt.Sprintf("first_%d_", time.Now().UnixNano())
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch := prefix + fmt.Sprint(n.Add(1))
			if _, _, err := e.History(ch, benchReadOpts); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.StopTimer()
	b.ReportMetric(metaRows(b, e, prefix)/float64(b.N), "rows/op")
}

// BenchmarkPostgresStreamBroker_EmptyRepeatRead reads the position of empty
// channels which were read before.
func BenchmarkPostgresStreamBroker_EmptyRepeatRead(b *testing.B) {
	e, _, _ := newBenchPostgresStreamBroker(b)
	const numChannels = 1000
	prefix := fmt.Sprintf("repeat_%d_", time.Now().UnixNano())
	for i := 0; i < numChannels; i++ {
		_, _, err := e.History(prefix+fmt.Sprint(i), benchReadOpts)
		require.NoError(b, err)
	}
	var n atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch := prefix + fmt.Sprint(n.Add(1)%numChannels)
			if _, _, err := e.History(ch, benchReadOpts); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPostgresStreamBroker_PublishedRead reads the position of channels
// with publications.
func BenchmarkPostgresStreamBroker_PublishedRead(b *testing.B) {
	e, _, _ := newBenchPostgresStreamBroker(b)
	const numChannels = 1000
	prefix := fmt.Sprintf("published_%d_", time.Now().UnixNano())
	for i := 0; i < numChannels; i++ {
		_, err := e.Publish(prefix+fmt.Sprint(i), []byte("x"), centrifuge.PublishOptions{HistorySize: 10, HistoryTTL: time.Hour})
		require.NoError(b, err)
	}
	var n atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch := prefix + fmt.Sprint(n.Add(1)%numChannels)
			if _, _, err := e.History(ch, benchReadOpts); err != nil {
				b.Fatal(err)
			}
		}
	})
}
