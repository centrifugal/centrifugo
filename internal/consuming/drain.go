package consuming

import (
	"context"
	"sync"
)

// drain coordinates Run and Close of a consumer. After Run's context is
// cancelled, messages in flight are handled and acknowledged with ctx, which
// is cancelled only when Close stops waiting for Run.
type drain struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	started bool
	closed  bool
	done    chan struct{}
}

func newDrain() *drain {
	ctx, cancel := context.WithCancel(context.Background())
	return &drain{ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

// runStarted marks Run as started. The returned function must be called when
// Run returns. It returns false when Close was called before: Run must then
// return at once.
func (d *drain) runStarted() (func(), bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.started {
		return nil, false
	}
	d.started = true
	return func() { close(d.done) }, true
}

// wait waits until Run returned (if it started) or ctx is done, then cancels
// the drain context. It reports whether Run started.
func (d *drain) wait(ctx context.Context) (bool, error) {
	d.mu.Lock()
	d.closed = true
	started := d.started
	d.mu.Unlock()
	var err error
	if started {
		select {
		case <-d.done:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	d.cancel()
	return started, err
}

// handlingContext returns ctx while it is not cancelled. Once it is, it
// returns a context with the values of ctx and the cancellation of the drain
// context, so messages in flight are still handled.
func (d *drain) handlingContext(ctx context.Context) context.Context {
	if ctx.Err() == nil {
		return ctx
	}
	return valuesContext{Context: d.ctx, values: ctx}
}

// valuesContext has the deadline and cancellation of Context and the values
// of values.
type valuesContext struct {
	context.Context
	values context.Context
}

func (c valuesContext) Value(key any) any {
	return c.values.Value(key)
}
