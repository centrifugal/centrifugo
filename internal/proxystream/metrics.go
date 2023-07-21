package proxystream

import (
	"github.com/prometheus/client_golang/prometheus"
)

var metricsNamespace = "centrifugo"

var (
	proxyCallTotalCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: "proxy_stream",
		Name:      "count",
		Help:      "Proxy stream call total count.",
	}, []string{"name", "type"})
	proxyCallErrorCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: "proxy_stream",
		Name:      "errors",
		Help:      "Proxy stream call error count.",
	}, []string{"name", "type", "error"})
)

func init() {
	prometheus.MustRegister(proxyCallTotalCount)
	prometheus.MustRegister(proxyCallErrorCount)
}
