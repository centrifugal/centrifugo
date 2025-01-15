package api

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var metricsNamespace = "centrifugo"

var (
	apiCommandErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: "api",
		Name:      "command_errors_total",
		Help:      "Total errors in API commands.",
	}, []string{"protocol", "method", "error"})
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
	rpcDurationSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  metricsNamespace,
		Subsystem:  "api",
		Name:       "rpc_duration_seconds",
		Objectives: map[float64]float64{0.5: 0.05, 0.99: 0.001, 0.999: 0.0001},
		Help:       "Duration of API per command.",
	}, []string{"protocol", "method"})
)

func init() {
	prometheus.MustRegister(apiCommandErrorsTotal)
	prometheus.MustRegister(apiCommandDurationSummary)
	prometheus.MustRegister(apiCommandDurationHistogram)
	prometheus.MustRegister(rpcDurationSummary)
}

func incError(protocol string, method string, code uint32) {
	apiCommandErrorsTotal.WithLabelValues(protocol, method, strconv.FormatUint(uint64(code), 10)).Inc()
}

func incErrorStringCode(protocol string, method string, code string) {
	apiCommandErrorsTotal.WithLabelValues(protocol, method, code).Inc()
}

func observe(started time.Time, protocol string, method string) {
	duration := time.Since(started).Seconds()
	apiCommandDurationSummary.WithLabelValues(protocol, method).Observe(duration)
	apiCommandDurationHistogram.WithLabelValues(protocol, method).Observe(duration)
}

func observeRPC(started time.Time, protocol string, method string) {
	duration := time.Since(started).Seconds()
	rpcDurationSummary.WithLabelValues(protocol, method).Observe(duration)
}
