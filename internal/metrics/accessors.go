package metrics

import "github.com/prometheus/client_golang/prometheus"

// Proxy metrics - exported for use by proxy package
var (
	ProxyCallDurationSummary   *prometheus.SummaryVec
	ProxyCallDurationHistogram *prometheus.HistogramVec
	ProxyCallErrorCount        *prometheus.CounterVec
	ProxyCallInflightRequests  *prometheus.GaugeVec
)

// API metrics - exported for use by api package
var (
	APICommandErrorsTotal       *prometheus.CounterVec
	APICommandDurationSummary   *prometheus.SummaryVec
	APICommandDurationHistogram *prometheus.HistogramVec
	RPCDurationSummary          *prometheus.SummaryVec
)

// Consumer metrics - exported for use by consuming package
var (
	ConsumerProcessedTotal *prometheus.CounterVec
	ConsumerErrorsTotal    *prometheus.CounterVec
)

// Middleware metrics - exported for use by middleware package
var (
	ConnLimitReached  prometheus.Counter
	HTTPRequestsTotal *prometheus.CounterVec
)
