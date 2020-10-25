package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// RPCHandlerConfig ...
type RPCHandlerConfig struct {
	Proxy RPCProxy
}

// RPCHandler ...
type RPCHandler struct {
	config    RPCHandlerConfig
	summary   prometheus.Observer
	histogram prometheus.Observer
	errors    prometheus.Counter
}

// NewRPCHandler ...
func NewRPCHandler(c RPCHandlerConfig) *RPCHandler {
	return &RPCHandler{
		config:    c,
		summary:   proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "rpc"),
		histogram: proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "rpc"),
		errors:    proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "rpc"),
	}
}

// RPCHandlerFunc ...
type RPCHandlerFunc func(*centrifuge.Client, centrifuge.RPCEvent) (centrifuge.RPCResult, error)

// Handle RPC.
func (h *RPCHandler) Handle(node *centrifuge.Node) RPCHandlerFunc {
	return func(client *centrifuge.Client, e centrifuge.RPCEvent) (centrifuge.RPCResult, error) {
		started := time.Now()
		rpcRep, err := h.config.Proxy.ProxyRPC(client.Context(), RPCRequest{
			Method:    e.Method,
			Data:      e.Data,
			ClientID:  client.ID(),
			UserID:    client.UserID(),
			Transport: client.Transport(),
		})
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.RPCResult{}, nil
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying RPC", map[string]interface{}{"error": err.Error()}))
			return centrifuge.RPCResult{}, centrifuge.ErrorInternal
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)
		if rpcRep.Disconnect != nil {
			return centrifuge.RPCResult{}, rpcRep.Disconnect
		}
		if rpcRep.Error != nil {
			return centrifuge.RPCResult{}, rpcRep.Error
		}

		rpcData := rpcRep.Result
		var data []byte
		if rpcData != nil {
			if client.Transport().Encoding() == "json" {
				data = rpcData.Data
			} else {
				if rpcData.Base64Data != "" {
					decodedData, err := base64.StdEncoding.DecodeString(rpcData.Base64Data)
					if err != nil {
						node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
						return centrifuge.RPCResult{}, centrifuge.ErrorInternal
					}
					data = decodedData
				}
			}
		}

		return centrifuge.RPCResult{
			Data: data,
		}, nil
	}
}
