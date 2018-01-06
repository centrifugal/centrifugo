package metrics

var DefaultRegistry *Registry

func init() {
	DefaultRegistry = NewRegistry()
}

// Registry contains various Centrifugo statistic and metric information aggregated
// once in a configurable interval.
type Registry struct {
	Gauges        *GaugeRegistry
	Counters      *CounterRegistry
	HDRHistograms *HDRHistogramRegistry
}

// NewRegistry initializes Registry.
func NewRegistry() *Registry {
	return &Registry{
		Counters:      NewCounterRegistry(),
		Gauges:        NewGaugeRegistry(),
		HDRHistograms: NewHDRHistogramRegistry(),
	}
}

// RegisterCounter allows to register counter in registry.
func (m *Registry) RegisterCounter(name string, c *Counter) {
	m.Counters.Register(name, c)
}

// RegisterGauge allows to register gauge in registry.
func (m *Registry) RegisterGauge(name string, g *Gauge) {
	m.Gauges.Register(name, g)
}

// RegisterHDRHistogram allows to register hdr histogram in registry.
func (m *Registry) RegisterHDRHistogram(name string, h *HDRHistogram) {
	m.HDRHistograms.Register(name, h)
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
