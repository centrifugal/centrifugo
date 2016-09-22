package metrics

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

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
