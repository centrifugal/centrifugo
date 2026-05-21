//go:build integration

package pgoutbox

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// These tests exercise the LockWorker against a real Postgres. They are
// gated by the `integration` build tag and the CENTRIFUGE_POSTGRES_URL env
// var (matching the other pg-integration suites in this repo).

func getPostgresConnString(tb testing.TB) string {
	tb.Helper()
	dsn := os.Getenv("CENTRIFUGE_POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://test:test@localhost:5432/test?sslmode=disable"
	}
	return dsn
}

// newLockTestPool returns a pool sized for the lock-conn-ping test:
// 4 max conns so there's room for the lock conn + the polling pool's needs
// + a test-helper conn used to kill the lock conn from outside.
func newLockTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(getPostgresConnString(t))
	require.NoError(t, err)
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// findLockHolderPID returns the backend pid of the session holding our
// advisory lock, or 0 if not found. Used to identify the lock conn so we
// can terminate it from a different session.
func findLockHolderPID(t *testing.T, pool *pgxpool.Pool, lockID int64) int32 {
	t.Helper()
	var pid int32
	err := pool.QueryRow(context.Background(), `
		SELECT pid FROM pg_locks
		WHERE locktype = 'advisory'
		  AND ((classid::bigint << 32) | objid::bigint::int8) = $1
		LIMIT 1
	`, lockID).Scan(&pid)
	if err != nil {
		return 0
	}
	return pid
}

// TestLockWorker_LockConnTerminationDetected is the regression test for the
// advisory-lock health-check ping. Once the worker has acquired the advisory
// lock, an external session terminates the lock-holding backend; Postgres
// then auto-releases the advisory lock. The ping during the next idle wait
// must detect the dead session and surface it via ErrorFn so the worker
// releases the (already-gone) lock and re-acquires it via the outer loop.
//
// Without the ping, the worker would happily continue polling on a healthy
// PollPool connection while another node could have already acquired the
// advisory lock — dual-leader processing of the same outbox region.
func TestLockWorker_LockConnTerminationDetected(t *testing.T) {
	pool := newLockTestPool(t)
	// Randomise the lock ID per test run so concurrent test packages don't
	// collide on the same advisory lock.
	lockID := rand.Int64()

	var (
		processCalls atomic.Int32
		errs         []error
		errsMu       sync.Mutex
	)

	lw := &LockWorker{
		PollPool:      pool,
		LockPool:      pool,
		ShardIDs:      []int{0},
		LockID:        lockID,
		PollInterval:  100 * time.Millisecond,
		RetryInterval: 100 * time.Millisecond,
		InitCursor: func(ctx context.Context, _ *pgxpool.Pool) (int64, error) {
			return 0, nil
		},
		ProcessBatch: func(ctx context.Context, _ *pgxpool.Pool, _ int64, _ []int) (int, int64, error) {
			processCalls.Add(1)
			return 0, 0, nil // always idle
		},
		ErrorFn: func(msg string, err error) {
			errsMu.Lock()
			defer errsMu.Unlock()
			errs = append(errs, fmt.Errorf("%s: %w", msg, err))
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closeCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		lw.Run(ctx, closeCh)
	}()
	t.Cleanup(func() {
		close(closeCh)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not exit on close")
		}
	})

	// Wait until the worker has acquired the lock.
	var pid int32
	require.Eventually(t, func() bool {
		pid = findLockHolderPID(t, pool, lockID)
		return pid != 0
	}, 5*time.Second, 50*time.Millisecond, "worker never acquired the advisory lock")

	// Externally terminate the lock-holding session — Postgres auto-releases
	// the advisory lock on session end.
	_, err := pool.Exec(context.Background(), `SELECT pg_terminate_backend($1)`, pid)
	require.NoError(t, err)

	// Within a few PollInterval ticks the ping must fail and the worker must
	// log it via ErrorFn. The exact error from pgx may vary by version, so
	// we only require some error to be reported referencing the ping path
	// or session loss.
	require.Eventually(t, func() bool {
		errsMu.Lock()
		defer errsMu.Unlock()
		for _, e := range errs {
			s := e.Error()
			if containsAny(s, "ping failed", "release advisory lock") {
				return true
			}
		}
		return false
	}, 3*time.Second, 100*time.Millisecond, "ping failure was not surfaced via ErrorFn")

	// And the worker should be able to re-acquire the lock under a fresh
	// connection on its next cycle. We don't assert specific PIDs because
	// the new lock holder may use any conn from the pool.
	require.Eventually(t, func() bool {
		newPID := findLockHolderPID(t, pool, lockID)
		return newPID != 0 && newPID != pid
	}, 5*time.Second, 100*time.Millisecond, "worker did not re-acquire the advisory lock after lock-conn loss")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if idx := indexOfStr(s, sub); idx >= 0 {
			return true
		}
	}
	return false
}

func indexOfStr(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
