package proxy

import (
	"github.com/prometheus/client_golang/prometheus"
)

var metricsNamespace = "centrifugo"

var (
	proxyCallDurationSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  metricsNamespace,
		Subsystem:  "proxy",
		Name:       "duration_seconds",
		Objectives: map[float64]float64{0.5: 0.05, 0.99: 0.001, 0.999: 0.0001},
		Help:       "Duration of proxy call.",
	}, []string{"protocol", "type"})
	proxyCallDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: "proxy",
		Name:      "duration_seconds_histogram",
		Buckets:   prometheus.DefBuckets,
		Help:      "Histogram of duration of proxy call.",
	}, []string{"protocol", "type"})
	proxyCallErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: "proxy",
		Name:      "errors",
		Help:      "Proxy call error count.",
	}, []string{"protocol", "type"})
)

func init() {
	prometheus.MustRegister(proxyCallDurationSummary)
	prometheus.MustRegister(proxyCallDurationHistogram)
	prometheus.MustRegister(proxyCallErrorCount)
}
