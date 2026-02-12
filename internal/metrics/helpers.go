package metrics

import (
	"strconv"
	"time"
)

// API metric helper functions - these were previously in the api package

// IncAPIError increments the API error counter with a numeric error code.
func IncAPIError(protocol string, method string, code uint32) {
	APICommandErrorsTotal.WithLabelValues(protocol, method, strconv.FormatUint(uint64(code), 10)).Inc()
}

// IncAPIErrorStringCode increments the API error counter with a string error code.
func IncAPIErrorStringCode(protocol string, method string, code string) {
	APICommandErrorsTotal.WithLabelValues(protocol, method, code).Inc()
}

// ObserveAPICommand observes the duration of an API command.
func ObserveAPICommand(started time.Time, protocol string, method string) {
	duration := time.Since(started).Seconds()
	APICommandDurationSummary.WithLabelValues(protocol, method).Observe(duration)
	APICommandDurationHistogram.WithLabelValues(protocol, method).Observe(duration)
}

// ObserveRPC observes the duration of an RPC call.
func ObserveRPC(started time.Time, protocol string, method string) {
	duration := time.Since(started).Seconds()
	RPCDurationSummary.WithLabelValues(protocol, method).Observe(duration)
}

// Consumer metric helper functions - these were previously in the consuming package

// InitConsumerMetrics initializes consumer metrics with zero values for the given consumer name.
func InitConsumerMetrics(consumerName string) {
	ConsumerProcessedTotal.WithLabelValues(consumerName).Add(0)
	ConsumerErrorsTotal.WithLabelValues(consumerName).Add(0)
}
