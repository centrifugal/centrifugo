package api

import (
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
)

func init() {
	prometheus.MustRegister(apiCommandDurationSummary)
}
