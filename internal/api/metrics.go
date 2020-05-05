package api

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var metricsNamespace = "centrifugo"

var (
	apiCommandDurationSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  metricsNamespace,
		Subsystem:  "api",
		Name:       "command_duration_seconds",
		Objectives: map[float64]float64{0.5: 0.05, 0.99: 0.001, 0.999: 0.0001},
		Help:       "Duration of API per command.",
	}, []string{"protocol", "method"})
	apiCommandDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: "api",
		Buckets:   prometheus.DefBuckets,
		Name:      "command_duration_seconds_histogram",
		Help:      "Histogram of duration of API per command.",
	}, []string{"protocol", "method"})
)

func init() {
	prometheus.MustRegister(apiCommandDurationSummary)
	prometheus.MustRegister(apiCommandDurationHistogram)
}

func observe(started time.Time, protocol string, method string) {
	duration := time.Since(started).Seconds()
	apiCommandDurationSummary.WithLabelValues(protocol, method).Observe(duration)
	apiCommandDurationHistogram.WithLabelValues(protocol, method).Observe(duration)
}
