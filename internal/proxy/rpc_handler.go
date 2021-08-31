package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"

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
type RPCHandlerFunc func(*centrifuge.Client, centrifuge.RPCEvent) (centrifuge.RPCReply, error)

// Handle RPC.
func (h *RPCHandler) Handle(node *centrifuge.Node) RPCHandlerFunc {
	return func(client *centrifuge.Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error) {
		started := time.Now()

		req := &proxyproto.RPCRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(h.config.Proxy.UseBase64()),

			User:   client.UserID(),
			Method: e.Method,
		}
		if h.config.Proxy.IncludeMeta() {
			if connMeta, ok := clientcontext.GetContextConnectionMeta(client.Context()); ok {
				req.Meta = proxyproto.Raw(connMeta.Meta)
			}
		}
		if !h.config.Proxy.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}

		rpcRep, err := h.config.Proxy.ProxyRPC(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.RPCReply{}, nil
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying RPC", map[string]interface{}{"error": err.Error()}))
			return centrifuge.RPCReply{}, centrifuge.ErrorInternal
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)
		if rpcRep.Disconnect != nil {
			return centrifuge.RPCReply{}, proxyproto.DisconnectFromProto(rpcRep.Disconnect)
		}
		if rpcRep.Error != nil {
			return centrifuge.RPCReply{}, proxyproto.ErrorFromProto(rpcRep.Error)
		}

		rpcData := rpcRep.Result
		var data []byte
		if rpcData != nil {
			if rpcData.B64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(rpcData.B64Data)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.RPCReply{}, centrifuge.ErrorInternal
				}
				data = decodedData
			} else {
				data = rpcData.Data
			}
		}

		return centrifuge.RPCReply{
			Data: data,
		}, nil
	}
}
