//go:build integration

package pgemptymeta

import (
	"context"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/pgtest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// The channel had no row when a run queried, and a subscriber's read created
// it and scheduled the channel before the run applied what it saw.
func TestKeeperScheduleDuringQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, pgtest.SchemaDSN(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `CREATE TABLE meta (channel TEXT PRIMARY KEY, top_offset BIGINT, expires_at TIMESTAMPTZ, updated_at TIMESTAMPTZ)`)
	require.NoError(t, err)

	k := New(pool, "meta", func(string) bool { return true })
	now := time.Now()
	k.Schedule("ch", 0, now)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	var pid int
	require.NoError(t, tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	_, err = tx.Exec(ctx, "LOCK TABLE meta IN EXCLUSIVE MODE")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- k.Keep(ctx, now) }()
	require.Eventually(t, func() bool {
		var waiting bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int = ANY(pg_blocking_pids(pid)))", pid).Scan(&waiting)
		return err == nil && waiting
	}, 10*time.Second, 10*time.Millisecond, "the run did not reach its query")
	k.Schedule("ch", 60, now)
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, <-done)

	require.True(t, k.IsDue("ch", now.Add(time.Minute)), "the run dropped what was scheduled during it")
}
