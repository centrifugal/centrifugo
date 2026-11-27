package pgemptymeta

import (
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Lazy is a Keeper made and started on first use: the first channel with an
// initial meta TTL (HistoryMetaTTLInitial globally, per namespace or per map).
// Without one anywhere it is never made, and a broker behaves as before the
// option: reads alone extend the metas of empty channels.
type Lazy struct {
	once    sync.Once
	started atomic.Bool
	keeper  *Keeper
	newFn   func() *Keeper
	start   func(*Keeper)
}

// NewLazy returns a Lazy whose Keeper start runs, in a worker of the broker.
func NewLazy(pool *pgxpool.Pool, table string, hasSubscribers func(ch string) bool, start func(*Keeper)) *Lazy {
	return &Lazy{
		newFn: func() *Keeper { return New(pool, table, hasSubscribers) },
		start: start,
	}
}

// Get returns the Keeper, making and starting it on first use.
func (l *Lazy) Get() *Keeper {
	l.once.Do(func() {
		l.keeper = l.newFn()
		l.started.Store(true)
		l.start(l.keeper)
	})
	return l.keeper
}

// Started returns the Keeper if it was made, nil otherwise (whose methods do
// nothing).
func (l *Lazy) Started() *Keeper {
	if !l.started.Load() {
		return nil
	}
	return l.keeper
}
