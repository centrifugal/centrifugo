package metrics

import (
	"sync/atomic"
)

type Gauge struct {
	value int64
}

func NewGauge() *Gauge {
	return &Gauge{}
}

// Set
func (g *Gauge) Set(value int64) {
	atomic.StoreInt64(&g.value, value)
}

// Load
func (g *Gauge) Load() int64 {
	return atomic.LoadInt64(&g.value)
}

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
