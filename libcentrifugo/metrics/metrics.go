package metrics

import (
	"sync"
)

var Metrics *Registry

func init() {
	Metrics = NewRegistry()
}

// Registry contains various Centrifugo statistic and metric information aggregated
// once in a configurable interval.
type Registry struct {
	Gauges        *GaugeRegistry
	Counters      *CounterRegistry
	HDRHistograms *HDRHistogramRegistry

	// mu protects from multiple processes updating snapshot values at once
	// but raw counters may still increment atomically while held so it's not a strict
	// point-in-time snapshot of all values.
	mu sync.Mutex
}

func NewRegistry() *Registry {
	return &Registry{
		Counters:      NewCounterRegistry(),
		Gauges:        NewGaugeRegistry(),
		HDRHistograms: NewHDRHistogramRegistry(),
	}
}

func (m *Registry) RegisterCounter(name string, c *Counter) {
	m.Counters.Register(name, c)
}

func (m *Registry) RegisterGauge(name string, g *Gauge) {
	m.Gauges.Register(name, g)
}

func (m *Registry) RegisterHDRHistogram(name string, h *HDRHistogram) {
	m.HDRHistograms.Register(name, h)
}

func (m *Registry) UpdateSnapshot() {
	// We update under a lock to ensure that no other process is also updating
	// snapshot nor copying the values with GetRawMetrics/GetSnapshotMetrics.
	// Other processes CAN still atomically increment raw counter values while we
	// go though - we don't guarantee counter values are point-in-time consistent
	// with each other
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters.UpdateDelta()
	m.HDRHistograms.Rotate()
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
