package metrics

import "github.com/prometheus/client_golang/prometheus"

// Proxy metrics - exported for use by proxy package.
//
// ProxyCallDurationSummary holds the legacy Summary instrument by default;
// when prometheus.native_histograms is enabled it is a no-op observer (see
// internal noopObserverVec), so callers can continue to construct cached
// observers and call .Observe() without nil-checks. ProxyCallDurationHistogram
// is the canonical companion — prefer it for new queries and OTel export.
var (
	ProxyCallDurationSummary   prometheus.ObserverVec
	ProxyCallDurationHistogram *prometheus.HistogramVec
	ProxyCallErrorCount        *prometheus.CounterVec
	ProxyCallInflightRequests  *prometheus.GaugeVec
)

// API metrics - exported for use by api package.
//
// APICommandDurationSummary follows the same pattern as ProxyCallDurationSummary
// (real Summary by default, no-op when native_histograms is enabled).
// APICommandDurationHistogram is the canonical companion.
//
// RPCDurationSummary follows the same pattern. RPCDurationHistogram is the
// companion Histogram exposed as "rpc_duration_seconds_histogram" —
// unconditional, with native (sparse, exponential) schema when
// native_histograms is enabled.
var (
	APICommandErrorsTotal       *prometheus.CounterVec
	APICommandDurationSummary   prometheus.ObserverVec
	APICommandDurationHistogram *prometheus.HistogramVec
	RPCDurationSummary          prometheus.ObserverVec
	RPCDurationHistogram        *prometheus.HistogramVec
)

// Consumer metrics - exported for use by consuming package
var (
	ConsumerProcessedTotal *prometheus.CounterVec
	ConsumerErrorsTotal    *prometheus.CounterVec
)

// Shared poll proxy metrics - exported for use by proxy package
var (
	SharedPollProxyRequestItems  *prometheus.HistogramVec
	SharedPollProxyResponseItems *prometheus.HistogramVec
)

// Middleware metrics - exported for use by middleware package
var (
	ConnLimitReached  prometheus.Counter
	HTTPRequestsTotal *prometheus.CounterVec
)

// PostgreSQL broker metrics - exported for use by pgmapbroker and pgstreambroker.
// Shared subsystem "pg_broker" with a "broker" label to distinguish map vs stream.
var (
	PGBrokerCleanupRowsDeletedTotal *prometheus.CounterVec
	PGBrokerOutboxCursorLagSeconds  *prometheus.GaugeVec
	PGBrokerPartitions              *prometheus.GaugeVec
)
