package pgoutbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Worker runs the plain outbox poll loop for a single shard.
//
// At startup Worker calls InitCursor once (retrying in-place on error
// until success or shutdown). Then it drains ProcessBatch until processed
// reaches zero, and waits on NotifyCh / PollInterval / shutdown signals
// before draining again.
//
// Error policy: a ProcessBatch error is logged via ErrorFn and the worker
// falls through to the idle wait, then resumes on the next iteration. This
// is log-and-continue semantics — a transient query error should not cause
// the worker to stop. For fail-fast semantics that drops an advisory lock,
// use LockWorker instead.
type Worker struct {
	// Pool is the pgxpool used for cursor bootstrap and batch polling.
	// Typically a read-replica pool when replicas are configured.
	Pool *pgxpool.Pool

	// ShardIDs is the set of shard ids this worker owns. Passed through
	// to ProcessBatch so the callback can scope its SELECT WHERE clause.
	// Typically a single-element slice equal to this worker's index.
	ShardIDs []int

	// PollInterval is the idle wait between drain passes when NotifyCh
	// is not set or does not fire.
	PollInterval time.Duration

	// NotifyCh is an optional wakeup channel driven by a LISTEN/NOTIFY
	// loop. A non-blocking send on this channel wakes the idle wait.
	// nil is fine — a nil channel in a select case is runtime-inert
	// (blocks forever, never fires). NotifyCh must never be closed
	// by the sender; see package doc for the invariant.
	NotifyCh <-chan struct{}

	// InitCursor is called once at startup (with in-place retry on error)
	// to bootstrap the cursor value. Typically implemented as
	// SELECT COALESCE(MAX(id), 0) FROM <stream_table> on the Pool.
	InitCursor func(ctx context.Context, pool *pgxpool.Pool) (int64, error)

	// ProcessBatch fetches and delivers the next batch of rows whose id
	// is greater than cursor, filtered to the given shards. Returns the
	// number of rows processed and the maximum id seen. Returning
	// processed == 0 signals idle (the worker goes to wait).
	ProcessBatch func(ctx context.Context, pool *pgxpool.Pool, cursor int64, shardIDs []int) (processed int, maxID int64, err error)

	// ErrorFn is called for non-ctx errors. Must be safe for concurrent use.
	ErrorFn func(msg string, err error)
}

// Run starts the poll loop and returns when ctx is cancelled or closeCh is
// closed. Run is intended to be called in its own goroutine.
func (w *Worker) Run(ctx context.Context, closeCh <-chan struct{}) {
	cursor, ok := w.initCursorWithRetry(ctx, closeCh)
	if !ok {
		return
	}

	for {
		select {
		case <-closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Drain batches until idle.
		idle := true
		for {
			processed, maxID, err := w.ProcessBatch(ctx, w.Pool, cursor, w.ShardIDs)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				w.ErrorFn("outbox worker: process batch", err)
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

		// If we made progress, loop immediately to try for more.
		if !idle {
			continue
		}

		// Idle wait — NotifyCh may be nil (inert), time.After provides the
		// fallback poll interval, and closeCh/ctx.Done exit the worker.
		select {
		case <-closeCh:
			return
		case <-ctx.Done():
			return
		case <-w.NotifyCh:
		case <-time.After(w.PollInterval):
		}
	}
}

// initCursorWithRetry calls InitCursor in a loop, retrying with a 1s delay
// on any non-ctx error. Returns (cursor, true) on success, or (0, false)
// if the context was cancelled or closeCh was closed before a successful
// init. Unlike the previous self-respawn pattern in pgmapbroker, this
// retries in-place in the current goroutine.
func (w *Worker) initCursorWithRetry(ctx context.Context, closeCh <-chan struct{}) (int64, bool) {
	const retryDelay = time.Second
	for {
		cursor, err := w.InitCursor(ctx, w.Pool)
		if err == nil {
			return cursor, true
		}
		if ctx.Err() != nil {
			return 0, false
		}
		w.ErrorFn("outbox worker: init cursor", err)
		select {
		case <-closeCh:
			return 0, false
		case <-ctx.Done():
			return 0, false
		case <-time.After(retryDelay):
		}
	}
}
