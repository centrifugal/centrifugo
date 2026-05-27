package pgoutbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationListener runs a PostgreSQL LISTEN loop, non-blocking-sending
// on NotifyCh each time a notification is received on Channel.
//
// Lifecycle:
//  1. Acquire a pinned connection from Pool.
//  2. LISTEN <Channel>.
//  3. WaitForNotification in a loop. On each notification, non-blocking
//     send on NotifyCh (drops the signal if the channel is already full —
//     the receiver only needs to know "wake up", not "how many times").
//  4. On connection error: release, wait with exponential backoff (1s → 30s),
//     re-acquire.
//  5. On ctx cancel or closeCh close: release and return.
//
// NotifyCh is shared with one or more Worker instances. It must never be
// closed by the listener — a closed channel would cause Worker's idle
// wait select to spin. Workers use nil-safe select semantics so NotifyCh
// may also be nil if no listener is wired up.
type NotificationListener struct {
	// Pool is the pgxpool used to acquire LISTEN connections.
	Pool *pgxpool.Pool

	// Channel is the PostgreSQL notification channel name (bare,
	// unquoted). Typically set by the caller to match the pg_notify
	// channel name emitted by its stream insert triggers.
	Channel string

	// NotifyCh is the buffered wakeup channel. Sends are non-blocking;
	// if the channel is full the signal is dropped. Must not be closed.
	NotifyCh chan<- struct{}

	// ErrorFn is called on connection and LISTEN errors. Must be safe for
	// concurrent use (in practice this listener runs in one goroutine).
	ErrorFn func(msg string, err error)

	// OnReady, if set, is invoked after each successful LISTEN — i.e., once
	// the listener is bound and will deliver subsequent NOTIFY signals on
	// the channel. Fires again after each reconnect. Used by tests that
	// need to publish only after the listener is bound; the production
	// path treats early NOTIFYs as benign (PollInterval is the fallback).
	OnReady func()
}

// Run starts the listener loop. Returns when ctx is cancelled or closeCh
// is closed.
func (l *NotificationListener) Run(ctx context.Context, closeCh <-chan struct{}) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-closeCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		conn, err := l.Pool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			l.ErrorFn("notification listener: acquire connection", err)
			if !waitWithBackoff(ctx, closeCh, &backoff, maxBackoff) {
				return
			}
			continue
		}

		_, err = conn.Exec(ctx, "LISTEN "+l.Channel)
		if err != nil {
			conn.Release()
			if ctx.Err() != nil {
				return
			}
			l.ErrorFn("notification listener: LISTEN", err)
			if !waitWithBackoff(ctx, closeCh, &backoff, maxBackoff) {
				return
			}
			continue
		}

		backoff = time.Second

		if l.OnReady != nil {
			l.OnReady()
		}

		// Inner notification loop.
		for {
			_, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				conn.Release()
				if ctx.Err() != nil {
					return
				}
				l.ErrorFn("notification listener: wait", err)
				break // reconnect
			}

			// Non-blocking send to wake any waiting worker.
			select {
			case l.NotifyCh <- struct{}{}:
			default:
			}
		}

		if !waitWithBackoff(ctx, closeCh, &backoff, maxBackoff) {
			return
		}
	}
}

// waitWithBackoff sleeps for *backoff, doubling it up to maxBackoff.
// Returns true if the sleep completed, false on shutdown.
func waitWithBackoff(ctx context.Context, closeCh <-chan struct{}, backoff *time.Duration, maxBackoff time.Duration) bool {
	d := *backoff
	*backoff = min(*backoff*2, maxBackoff)
	select {
	case <-closeCh:
		return false
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
