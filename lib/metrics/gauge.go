package metrics

import (
	"sync/atomic"
)

// Gauge represents a current value of sth.
type Gauge struct {
	value int64
}

// NewGauge initializes Gauge.
func NewGauge() *Gauge {
	return &Gauge{}
}

// Set allows to set gauge value.
func (g *Gauge) Set(value int64) {
	atomic.StoreInt64(&g.value, value)
}

// Load allows to get gauge value.
func (g *Gauge) Load() int64 {
	return atomic.LoadInt64(&g.value)
}

// GaugeRegistry is a registry of gauges by name.
type GaugeRegistry struct {
	gauges map[string]*Gauge
}

// NewGaugeRegistry creates new GaugeRegistry.
func NewGaugeRegistry() *GaugeRegistry {
	return &GaugeRegistry{
		gauges: make(map[string]*Gauge),
	}
}

// Register allows to register Gauge in registry.
func (r *GaugeRegistry) Register(name string, c *Gauge) {
	r.gauges[name] = c
}

// Get allows to get Gauge from registry by name.
func (r *GaugeRegistry) Get(name string) *Gauge {
	return r.gauges[name]
}

// Set allows to set gauge value in registry by name.
func (r *GaugeRegistry) Set(name string, value int64) {
	r.gauges[name].Set(value)
}

// LoadValues allows to get union of gauge values over registered gauges.
func (r *GaugeRegistry) LoadValues(names ...string) map[string]int64 {
	values := make(map[string]int64)
	for name, g := range r.gauges {
		if len(names) > 0 && !stringInSlice(name, names) {
			continue
		}
		values[name] = g.Load()
	}
	return values
}
