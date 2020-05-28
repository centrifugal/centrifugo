package cancelctx

import (
	"context"
	"time"
)

type customCancelContext struct {
	context.Context
	ch <-chan struct{}
}

func (c customCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c customCancelContext) Done() <-chan struct{}       { return c.ch }
func (c customCancelContext) Err() error {
	select {
	case <-c.ch:
		return context.Canceled
	default:
		return nil
	}
}

// New returns a wrapper context around original context that will
// be canceled on channel close.
func New(ctx context.Context, ch <-chan struct{}) context.Context {
	return customCancelContext{Context: ctx, ch: ch}
}
