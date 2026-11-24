//go:build integration

package pgstreambroker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fillMeta inserts live and expired meta rows of channels without
// publications, named "<prefix>live:N" and "<prefix>expired:N".
func fillMeta(b *testing.B, e *PostgresStreamBroker, prefix string, live, expired int) {
	b.Helper()
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (channel, epoch, expires_at, updated_at)
		SELECT $1 || 'live:' || g, 'e', NOW() + interval '1 hour', NOW() FROM generate_series(1, $2) g`, e.names.meta), prefix, live)
	require.NoError(b, err)
	_, err = e.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (channel, epoch, expires_at, updated_at)
		SELECT $1 || 'expired:' || g, 'e', NOW() - interval '1 minute', NOW() - interval '1 hour' FROM generate_series(1, $2) g`, e.names.meta), prefix, expired)
	require.NoError(b, err)
	_, err = e.pool.Exec(ctx, "ANALYZE "+e.names.meta)
	require.NoError(b, err)
}

// BenchmarkPostgresStreamBroker_EmptyMetaKeep runs one keeper pass over the
// subscribed empty channels of a node, all due: "extend" when their rows have
// less than half of their TTL left and are updated, "check" when they don't
// and are only queried, which is the usual case.
func BenchmarkPostgresStreamBroker_EmptyMetaKeep(b *testing.B) {
	for _, channels := range []int{1000, 10000} {
		for _, mode := range []string{"check", "extend"} {
			b.Run(fmt.Sprintf("channels_%d/%s", channels, mode), func(b *testing.B) {
				e, _, _ := newBenchPostgresStreamBroker(b)
				ctx := context.Background()
				_, err := e.pool.Exec(ctx, "DELETE FROM "+e.names.meta)
				require.NoError(b, err)
				fillMeta(b, e, "k_", channels, 0)
				k := e.emptyMeta.Get()
				k.HasSubscribers = func(string) bool { return true }
				names := make([]string, channels)
				for i := range names {
					names[i] = fmt.Sprintf("k_live:%d", i+1)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					if mode == "extend" {
						_, err := e.pool.Exec(ctx, fmt.Sprintf(
							`UPDATE %s SET expires_at = NOW() + interval '20 minutes', updated_at = NOW() - interval '40 minutes'`, e.names.meta))
						require.NoError(b, err)
					}
					now := time.Now()
					for _, ch := range names {
						k.Schedule(ch, 0, now)
					}
					b.StartTimer()
					require.NoError(b, k.Keep(ctx, now))
				}
				b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N*channels), "µs/channel")
			})
		}
	}
}

// BenchmarkPostgresStreamBroker_EmptyMetaKeepTableSize runs a keeper pass over
// 1000 subscribed empty channels (one batch, not due) with other rows in the
// meta table: a pass shouldn't depend on the table size.
func BenchmarkPostgresStreamBroker_EmptyMetaKeepTableSize(b *testing.B) {
	for _, other := range []int{0, 100000, 1000000} {
		b.Run(fmt.Sprintf("other_%d", other), func(b *testing.B) {
			e, _, _ := newBenchPostgresStreamBroker(b)
			ctx := context.Background()
			_, err := e.pool.Exec(ctx, "DELETE FROM "+e.names.meta)
			require.NoError(b, err)
			fillMeta(b, e, "other_", other, 0)
			fillMeta(b, e, "k_", 1000, 0)
			k := e.emptyMeta.Get()
			k.HasSubscribers = func(string) bool { return true }
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				now := time.Now()
				for j := 1; j <= 1000; j++ {
					k.Schedule(fmt.Sprintf("k_live:%d", j), 0, now)
				}
				b.StartTimer()
				require.NoError(b, k.Keep(ctx, now))
			}
		})
	}
}
