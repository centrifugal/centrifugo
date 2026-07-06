package pgoutbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// lockPingMinTimeout is the floor for the lock-connection ping timeout.
// PollInterval can be set very small (sub-second) to make idle polling
// responsive, but a healthy ping must still tolerate transient latency
// spikes — otherwise we spuriously release and re-acquire the lock.
const lockPingMinTimeout = 2 * time.Second

// lockLivenessInterval bounds how often the lock-holding connection is pinged
// for liveness, whether the worker is busy or idle. It caps how long a silently
// dropped lock connection — and therefore a dual-leader window — can go
// undetected under a sustained backlog (where the worker never goes idle).
const lockLivenessInterval = 5 * time.Second

// LockWorker runs the advisory-lock variant of the outbox poll loop.
//
// Unlike Worker, LockWorker holds a session-scoped PostgreSQL advisory
// lock for the lifetime of each poll session. This guarantees at most one
// active poller per shard across a cluster of Centrifugo nodes, which is
// required when the caller fans publications out via an inner Broker
// (Redis/NATS) — otherwise multiple nodes would deliver duplicates.
//
// Lifecycle per session:
//  1. Acquire a pinned connection from LockPool (typically the primary).
//  2. pg_try_advisory_lock(LockID) — non-blocking. On failure, release and
//     retry after RetryInterval.
//  3. InitCursor on PollPool (typically a read-replica) — each session
//     re-initializes from the current MAX(id) because the stream may have
//     advanced while another node held the lock.
//  4. Drain ProcessBatch until idle, then wait PollInterval, repeat.
//  5. On any ProcessBatch error: log via ErrorFn, release lock, retry from
//     step 1 after RetryInterval. (This is a harder recovery than Worker's
//     log-and-continue — a query error may indicate the lock-holding
//     connection is broken.)
//  6. On ctx cancel or closeCh close: release lock and return.
//
// LockWorker does NOT take a NotifyCh — the advisory-lock variant does
// not use LISTEN/NOTIFY today.
type LockWorker struct {
	// LockPool is the pool used to acquire the advisory lock. The lock is
	// session-scoped, so the connection is pinned for the lock's lifetime.
	// Typically the primary pool.
	LockPool *pgxpool.Pool

	// PollPool is the pool used for cursor bootstrap and batch polling
	// while the lock is held. Typically a read-replica pool.
	PollPool *pgxpool.Pool

	// ShardIDs is the set of shard ids this worker owns. Passed through
	// to ProcessBatch so the callback can scope its SELECT WHERE clause.
	ShardIDs []int

	// LockID is the advisory lock id. Must be globally unique per shard
	// across all LockWorker instances; callers typically derive it as
	// BaseID + shardIndex.
	LockID int64

	// PollInterval is the idle wait between drain passes.
	PollInterval time.Duration

	// RetryInterval is the wait between failed lock acquisition attempts
	// and between full poll-session retries after a ProcessBatch error.
	RetryInterval time.Duration

	// InitCursor is called once per lock acquisition (NOT once per
	// LockWorker lifetime) to bootstrap the cursor value from PollPool.
	// Typically implemented as SELECT COALESCE(MAX(id), 0) FROM <stream_table>.
	InitCursor func(ctx context.Context, pool *pgxpool.Pool) (int64, error)

	// ProcessBatch fetches and delivers the next batch. See Worker.ProcessBatch
	// for semantics. In LockWorker, a non-ctx error triggers lock release.
	ProcessBatch func(ctx context.Context, pool *pgxpool.Pool, cursor int64, shardIDs []int) (processed int, maxID int64, err error)

	// ErrorFn is called for non-ctx errors. Must be safe for concurrent use.
	ErrorFn func(msg string, err error)

	// InfoFn is called on successful lock acquisition. Optional.
	InfoFn func(msg string)
}

// Run starts the advisory-lock poll loop. Returns when ctx is cancelled
// or closeCh is closed.
func (lw *LockWorker) Run(ctx context.Context, closeCh <-chan struct{}) {
	for {
		select {
		case <-closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Acquire a connection from LockPool for the session-scoped lock.
		conn, err := lw.LockPool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			lw.ErrorFn("outbox worker: acquire lock connection", err)
			if !sleep(ctx, closeCh, lw.RetryInterval) {
				return
			}
			continue
		}

		// Non-blocking lock acquisition.
		var locked bool
		err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lw.LockID).Scan(&locked)
		if err != nil {
			conn.Release()
			if ctx.Err() != nil {
				return
			}
			lw.ErrorFn("outbox worker: try advisory lock", err)
			if !sleep(ctx, closeCh, lw.RetryInterval) {
				return
			}
			continue
		}

		if !locked {
			conn.Release()
			if !sleep(ctx, closeCh, lw.RetryInterval) {
				return
			}
			continue
		}

		if lw.InfoFn != nil {
			lw.InfoFn("outbox worker: acquired advisory lock")
		}

		// Lock acquired — init cursor and run the inner poll loop.
		shouldReturn := lw.runLockedSession(ctx, closeCh, conn)

		releaseAdvisoryLock(conn, lw.LockID, lw.ErrorFn)
		conn.Release()

		if shouldReturn {
			return
		}

		// Back off before retrying lock acquisition.
		if !sleep(ctx, closeCh, lw.RetryInterval) {
			return
		}
	}
}

// runLockedSession runs one locked poll session: initializes the cursor
// and drains batches until shutdown, error, or lock loss. Returns true if
// the caller should exit Run (ctx cancelled or closeCh closed); false if
// the caller should retry lock acquisition.
//
// The lock-holding conn is owned by the caller and released AFTER this
// method returns (via releaseAdvisoryLock + conn.Release); this method
// never releases it. We DO ping it during idle waits — see comment on the
// idle-wait branch below for why.
func (lw *LockWorker) runLockedSession(ctx context.Context, closeCh <-chan struct{}, lockConn *pgxpool.Conn) bool {
	// Initialize cursor from PollPool. Cursor is re-initialized on every
	// acquisition because the stream may have advanced while another node
	// held the lock.
	cursor, err := lw.InitCursor(ctx, lw.PollPool)
	if err != nil {
		if ctx.Err() != nil {
			return true
		}
		lw.ErrorFn("outbox worker: init cursor", err)
		return false
	}

	lastLockPing := time.Now()
	for {
		select {
		case <-closeCh:
			return true
		case <-ctx.Done():
			return true
		default:
		}

		// Drain batches until idle or error.
		idle := true
		lockLost := false
		for {
			processed, maxID, err := lw.ProcessBatch(ctx, lw.PollPool, cursor, lw.ShardIDs)
			if err != nil {
				if ctx.Err() != nil {
					return true
				}
				lw.ErrorFn("outbox worker: process batch", err)
				lockLost = true
				break
			}
			if processed == 0 {
				break
			}
			if maxID > cursor {
				cursor = maxID
			}
			idle = false
		}

		if lockLost {
			// Drop the session and re-acquire the lock; a query error may
			// indicate the lock-holding connection is broken.
			return false
		}

		// Ping the lock-holding connection on a time bound, whether the worker is
		// busy or idle. Postgres releases the advisory lock when this session dies,
		// so a silently reaped lock conn (pgbouncer reaping it, kernel TCP timeout,
		// a network partition affecting only this conn) lets another node acquire
		// the lock and dual-process. The lock conn is TCP-idle while queries run on
		// PollPool, and under a sustained backlog the worker never reaches the idle
		// wait below — so this liveness ping must run here, not only when idle. On
		// failure, drop the session and re-acquire, restoring single-writer
		// semantics.
		if time.Since(lastLockPing) >= lockLivenessInterval {
			alive, ctxDone := lw.pingLockConn(ctx, lockConn)
			if ctxDone {
				return true
			}
			if !alive {
				return false
			}
			lastLockPing = time.Now()
		}

		if !idle {
			continue
		}

		// Idle wait.
		select {
		case <-closeCh:
			return true
		case <-ctx.Done():
			return true
		case <-time.After(lw.PollInterval):
		}
	}
}

// pingLockConn checks the lock-holding connection is still alive. Returns
// (alive, ctxDone): alive=false means the connection is dead — Postgres has (or
// will) release the advisory lock, so the caller must drop the session and
// re-acquire; ctxDone=true means the worker is shutting down. The ping timeout
// is decoupled from PollInterval (which can be sub-second) with a floor so a
// transient latency spike doesn't spuriously drop the lock.
func (lw *LockWorker) pingLockConn(ctx context.Context, lockConn *pgxpool.Conn) (alive bool, ctxDone bool) {
	pingTimeout := lw.PollInterval
	if pingTimeout < lockPingMinTimeout {
		pingTimeout = lockPingMinTimeout
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	err := lockConn.Conn().Ping(pingCtx)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return false, true
		}
		lw.ErrorFn("outbox worker: lock connection ping failed — releasing lock", err)
		return false, false
	}
	return true, false
}

// releaseAdvisoryLock explicitly releases an advisory lock held on conn.
// Uses a fresh context.Background()-based timeout so it can still execute
// after the worker's context has been cancelled during shutdown. This is
// best-effort — the lock is also auto-released when the connection is
// returned to the pool and dropped.
func releaseAdvisoryLock(conn *pgxpool.Conn, lockID int64, errorFn func(msg string, err error)) {
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.Exec(releaseCtx, "SELECT pg_advisory_unlock($1)", lockID); err != nil {
		errorFn("outbox worker: release advisory lock", err)
	}
}

// sleep waits for the given duration or returns early on shutdown.
// Returns true if the sleep completed, false if ctx/closeCh triggered exit.
func sleep(ctx context.Context, closeCh <-chan struct{}, d time.Duration) bool {
	select {
	case <-closeCh:
		return false
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
