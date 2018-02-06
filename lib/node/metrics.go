package node

import (
	"github.com/prometheus/client_golang/prometheus"
)

var metricsNamespace = "centrifugo"
var metricsSubsystem = "node"

var (
	messagesSentCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "node_messages_sent_count",
		Help:      "TODO.",
	}, []string{"type"})

	messagesReceivedCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "node_messages_received_count",
		Help:      "TODO.",
	}, []string{"type"})

	actionCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "node_action_count",
		Help:      "TODO.",
	}, []string{"action"})

	numClientsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "node_num_clients",
		Help:      "TODO.",
	})

	numUsersGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "node_num_users",
		Help:      "TODO.",
	})

	numChannelsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "node_num_channels",
		Help:      "TODO.",
	})
)

func init() {
	prometheus.MustRegister(messagesSentCount)
	prometheus.MustRegister(messagesReceivedCount)
	prometheus.MustRegister(actionCount)
	prometheus.MustRegister(numClientsGauge)
	prometheus.MustRegister(numUsersGauge)
	prometheus.MustRegister(numChannelsGauge)
}
