package libcentrifugo

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCPUUsage(t *testing.T) {
	_, err := cpuUsage()
	assert.Equal(t, nil, err)
}

func AssertSameCounts(t *testing.T, when string, got, expected metricCounter) {
	if got.value != expected.value {
		t.Errorf("%s, raw counter value doesn't match: got %d expected %d", when, got.value, expected.value)
	}
	if got.lastIntervalValue != expected.lastIntervalValue {
		t.Errorf("%s, counter list interval value doesn't match: got %d expected %d", when,
			got.lastIntervalValue, expected.lastIntervalValue)
	}
	if got.lastIntervalDelta != expected.lastIntervalDelta {
		t.Errorf("%s, counter last interval delta doesn't match: got %d expected %d", when,
			got.lastIntervalDelta, expected.lastIntervalDelta)
	}
}

func TestMetrics(t *testing.T) {

	m := newMetricsRegistry()

	m.NumMsgPublished.Inc()
	m.NumMsgPublished.Inc()

	m.NumClientRequests.Add(10)

	// Deltas should all be zero as we didn't update yet
	AssertSameCounts(t, "Before update", m.NumMsgPublished, metricCounter{2, 0, 0, [5]int64{}})
	AssertSameCounts(t, "Before update", m.NumClientRequests, metricCounter{10, 0, 0, [5]int64{}})

	// Now update
	m.UpdateSnapshot()

	AssertSameCounts(t, "After update", m.NumMsgPublished, metricCounter{2, 2, 2, [5]int64{}})
	AssertSameCounts(t, "After update", m.NumClientRequests, metricCounter{10, 10, 10, [5]int64{}})

	// More increments
	m.NumMsgPublished.Inc()
	m.NumClientRequests.Inc()

	AssertSameCounts(t, "After second update", m.NumMsgPublished, metricCounter{3, 2, 2, [5]int64{}})
	AssertSameCounts(t, "After second update", m.NumClientRequests, metricCounter{11, 10, 10, [5]int64{}})

	// Second update
	m.UpdateSnapshot()

	AssertSameCounts(t, "After second update", m.NumMsgPublished, metricCounter{3, 3, 1, [5]int64{}})
	AssertSameCounts(t, "After second update", m.NumClientRequests, metricCounter{11, 11, 1, [5]int64{}})
}

func TestJSONMarshal(t *testing.T) {
	m := newMetricsRegistry()
	m.NumMsgPublished.Add(42)
	m.NumMsgQueued.Add(42)
	m.NumMsgSent.Add(42)
	m.NumAPIRequests.Add(42)
	m.NumClientRequests.Add(42)
	m.BytesClientIn.Add(42)
	m.BytesClientOut.Add(42)

	m.UpdateSnapshot()

	raw := m.GetRawMetrics()

	assert.Equal(t, int64(42), raw.NumMsgPublished)
	assert.Equal(t, int64(42), raw.NumMsgQueued)
	assert.Equal(t, int64(42), raw.NumMsgSent)
	assert.Equal(t, int64(42), raw.NumAPIRequests)
	assert.Equal(t, int64(42), raw.NumClientRequests)
	assert.Equal(t, int64(42), raw.BytesClientIn)
	assert.Equal(t, int64(42), raw.BytesClientOut)
	assert.Equal(t, int64(0), raw.TimeAPIMean)
	assert.Equal(t, int64(0), raw.TimeClientMax)
	assert.Equal(t, int64(0), raw.TimeClientMax)
	assert.Equal(t, int64(0), raw.TimeClientMean)

	// Update some values
	m.NumMsgSent.Add(42)
	m.NumAPIRequests.Add(42)
	m.NumClientRequests.Add(42)
	m.BytesClientIn.Add(42)

	// Now snapshot should be just the same since we've not updated
	snapshot := m.GetSnapshotMetrics()

	assert.Equal(t, int64(42), snapshot.NumMsgPublished)
	assert.Equal(t, int64(42), snapshot.NumMsgQueued)
	assert.Equal(t, int64(42), snapshot.NumMsgSent)
	assert.Equal(t, int64(42), snapshot.NumAPIRequests)
	assert.Equal(t, int64(42), snapshot.NumClientRequests)
	assert.Equal(t, int64(42), snapshot.BytesClientIn)
	assert.Equal(t, int64(42), snapshot.BytesClientOut)
	assert.Equal(t, int64(0), snapshot.TimeAPIMean)
	assert.Equal(t, int64(0), snapshot.TimeClientMax)
	assert.Equal(t, int64(0), snapshot.TimeClientMax)
	assert.Equal(t, int64(0), snapshot.TimeClientMean)

	// But Raw snapshot should include raw totals
	raw = m.GetRawMetrics()

	assert.Equal(t, int64(42), raw.NumMsgPublished)
	assert.Equal(t, int64(42), raw.NumMsgQueued)
	assert.Equal(t, int64(84), raw.NumMsgSent)
	assert.Equal(t, int64(84), raw.NumAPIRequests)
	assert.Equal(t, int64(84), raw.NumClientRequests)
	assert.Equal(t, int64(84), raw.BytesClientIn)
	assert.Equal(t, int64(42), raw.BytesClientOut)
	assert.Equal(t, int64(0), raw.TimeAPIMean)
	assert.Equal(t, int64(0), raw.TimeClientMax)
	assert.Equal(t, int64(0), raw.TimeClientMax)
	assert.Equal(t, int64(0), raw.TimeClientMean)

	// Now update
	m.UpdateSnapshot()

	// Now snapshot should have reset to only last round of increments
	snapshot = m.GetSnapshotMetrics()

	assert.Equal(t, int64(0), snapshot.NumMsgPublished)
	assert.Equal(t, int64(0), snapshot.NumMsgQueued)
	assert.Equal(t, int64(42), snapshot.NumMsgSent)
	assert.Equal(t, int64(42), snapshot.NumAPIRequests)
	assert.Equal(t, int64(42), snapshot.NumClientRequests)
	assert.Equal(t, int64(42), snapshot.BytesClientIn)
	assert.Equal(t, int64(0), snapshot.BytesClientOut)
	assert.Equal(t, int64(0), snapshot.TimeAPIMean)
	assert.Equal(t, int64(0), snapshot.TimeClientMax)
	assert.Equal(t, int64(0), snapshot.TimeClientMax)
	assert.Equal(t, int64(0), snapshot.TimeClientMean)

	// But Raw should still have all the totals (need to redefine it though since cpu and mem might change
	// during UpdateSnapshot above)
	raw = m.GetRawMetrics()

	assert.Equal(t, int64(42), raw.NumMsgPublished)
	assert.Equal(t, int64(42), raw.NumMsgQueued)
	assert.Equal(t, int64(84), raw.NumMsgSent)
	assert.Equal(t, int64(84), raw.NumAPIRequests)
	assert.Equal(t, int64(84), raw.NumClientRequests)
	assert.Equal(t, int64(84), raw.BytesClientIn)
	assert.Equal(t, int64(42), raw.BytesClientOut)
	assert.Equal(t, int64(0), raw.TimeAPIMean)
	assert.Equal(t, int64(0), raw.TimeClientMax)
	assert.Equal(t, int64(0), raw.TimeClientMax)
	assert.Equal(t, int64(0), raw.TimeClientMean)
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
