package metrics

import (
	"sync/atomic"
)

// Counter is a wrapper around a set of int64s that count things.
// It encapsulates both absolute monotonic counter (incremented atomically),
// and periodic delta which is updated every `app.config.NodeMetricsInterval`.
type Counter struct {
	value             int64
	lastIntervalValue int64
	lastIntervalDelta int64
	// Prevent false-sharing of consecutive counters in the parent registry
	// run go test -test.cpu 1,2,4,8 -test.bench=Atomic -test.run XXX
	// On my machine (quad core/8HT macbook) this is consistently ~60% faster with 8 threads.
	_padding [5]int64
}

// NewCounter creates new Counter.
func NewCounter() *Counter {
	return &Counter{}
}

// Value allows to get raw counter value.
func (c *Counter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

// IntervalValue allows to get last interval value for counter.
func (c *Counter) IntervalValue() int64 {
	return atomic.LoadInt64(&c.lastIntervalDelta)
}

// Inc is equivalent to Add(name, 1)
func (c *Counter) Inc() int64 {
	return c.Add(1)
}

// Add adds the given number to the counter and returns the new value.
func (c *Counter) Add(n int64) int64 {
	return atomic.AddInt64(&c.value, n)
}

// UpdateDelta updates the delta value for last interval based on current value and previous value.
func (c *Counter) UpdateDelta() {
	now := atomic.LoadInt64(&c.value)
	atomic.StoreInt64(&c.lastIntervalDelta, now-atomic.LoadInt64(&c.lastIntervalValue))
	atomic.StoreInt64(&c.lastIntervalValue, now)
}

// CounterRegistry contains counters with specified names.
type CounterRegistry struct {
	counters map[string]*Counter
}

// NewCounterRegistry creates new CounterRegistry.
func NewCounterRegistry() *CounterRegistry {
	return &CounterRegistry{
		counters: make(map[string]*Counter),
	}
}

// Register allows to register Counter in registry.
func (r *CounterRegistry) Register(name string, c *Counter) {
	r.counters[name] = c
}

// Get allows to get Counter from registry.
func (r *CounterRegistry) Get(name string) *Counter {
	return r.counters[name]
}

// Inc by name. Should only be called after registry already initialized.
func (r *CounterRegistry) Inc(name string) int64 {
	return r.counters[name].Inc()
}

// Add by name. Should only be called after registry already initialized.
func (r *CounterRegistry) Add(name string, n int64) int64 {
	return r.counters[name].Add(n)
}

// UpdateDelta updates snapshot counter values.
// Should only be called after registry already initialized.
func (r *CounterRegistry) UpdateDelta() {
	for _, counter := range r.counters {
		counter.UpdateDelta()
	}
}

// LoadValues allows to get union of raw counter values over registered counters.
// Should only be called after registry already initialized.
func (r *CounterRegistry) LoadValues(names ...string) map[string]int64 {
	values := make(map[string]int64)
	for name, c := range r.counters {
		if len(names) > 0 && !stringInSlice(name, names) {
			continue
		}
		values[name] = c.Value()
	}
	return values
}

// LoadIntervalValues allows to get union of last interval snapshot values over registered counters.
// Should only be called after registry already initialized.
func (r *CounterRegistry) LoadIntervalValues(names ...string) map[string]int64 {
	values := make(map[string]int64)
	for name, c := range r.counters {
		if len(names) > 0 && !stringInSlice(name, names) {
			continue
		}
		values[name] = c.IntervalValue()
	}
	return values
}
