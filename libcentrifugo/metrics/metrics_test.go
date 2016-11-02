package metrics

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {

	m := NewRegistry()

	m.RegisterCounter("num_msg_published", NewCounter())
	m.RegisterCounter("num_client_requests", NewCounter())

	m.Counters.Inc("num_msg_published")
	m.Counters.Inc("num_msg_published")

	m.Counters.Add("num_client_requests", 10)

	counter := m.Counters.Get("num_msg_published")
	assert.Equal(t, int64(2), counter.Value())

	counter = m.Counters.Get("num_client_requests")
	assert.Equal(t, int64(10), counter.Value())
}

/***********************************
 * Atomic false sharing benchmarks
 ***********************************/

type metricCounterNoPad struct {
	value, lastIntervalValue, lastIntervalDelta int64
}
type metricCounterWithPad struct {
	value, lastIntervalValue, lastIntervalDelta int64
	_padding                                    [5]int64 // pad rest of 64byte cache line
}

func doCountingNoPad(wg *sync.WaitGroup, n int) {
	for i := 0; i < n; i++ {
		atomic.AddInt64(&noPad[0].value, 1)
		atomic.AddInt64(&noPad[1].value, 1)
	}
	wg.Done()
}

func doCountingWithPad(wg *sync.WaitGroup, n int) {
	for i := 0; i < n; i++ {
		atomic.AddInt64(&withPad[0].value, 1)
		atomic.AddInt64(&withPad[1].value, 1)
	}
	wg.Done()
}

var noPad [2]metricCounterNoPad
var withPad [2]metricCounterWithPad

func BenchmarkAtomicCounterNoPad(b *testing.B) {
	var wg sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		wg.Add(1)
		go doCountingNoPad(&wg, b.N)
	}
	wg.Wait()
}

func BenchmarkAtomicCounterWithPad(b *testing.B) {
	var wg sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		wg.Add(1)
		go doCountingWithPad(&wg, b.N)
	}
	wg.Wait()
}
