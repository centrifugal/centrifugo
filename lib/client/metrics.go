package client

import (
	"github.com/prometheus/client_golang/prometheus"
)

var metricsNamespace = "centrifugo"
var metricsSubsystem = "client"

var (
	commandDurationSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  metricsNamespace,
		Subsystem:  metricsSubsystem,
		Name:       "command_duration_seconds",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		Help:       "TODO.",
	}, []string{"method"})
)

func init() {
	prometheus.MustRegister(commandDurationSummary)
}
