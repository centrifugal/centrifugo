package consuming

import "github.com/prometheus/client_golang/prometheus"

const metricsNamespace = "centrifugo"

// commonMetrics contains common metrics for all consumers to inherit. Consumers may
// provide their own metrics in addition to these.
type commonMetrics struct {
	processedTotal *prometheus.CounterVec
	errorsTotal    *prometheus.CounterVec
}

func newCommonMetrics(registry prometheus.Registerer) *commonMetrics {
	m := &commonMetrics{
		processedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "consumers",
			Name:      "messages_processed_total",
			Help:      "Total number of processed messages in consumer",
		}, []string{"consumer_name"}),
		errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "consumers",
			Name:      "errors_total",
			Help:      "Total number of errors in consumer",
		}, []string{"consumer_name"}),
	}
	registry.MustRegister(
		m.processedTotal,
		m.errorsTotal,
	)
	return m
}
