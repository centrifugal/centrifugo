package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// RPCHandlerConfig ...
type RPCHandlerConfig struct {
	Proxies map[string]RPCProxy
}

// RPCHandler ...
type RPCHandler struct {
	config    RPCHandlerConfig
	summary   map[string]prometheus.Observer
	histogram map[string]prometheus.Observer
	errors    map[string]prometheus.Counter
}

// NewRPCHandler ...
func NewRPCHandler(c RPCHandlerConfig) *RPCHandler {
	h := &RPCHandler{
		config: c,
	}
	summary := map[string]prometheus.Observer{}
	histogram := map[string]prometheus.Observer{}
	errors := map[string]prometheus.Counter{}
	for name, p := range c.Proxies {
		summary[name] = proxyCallDurationSummary.WithLabelValues(p.Protocol(), "rpc", name)
		histogram[name] = proxyCallDurationHistogram.WithLabelValues(p.Protocol(), "rpc", name)
		errors[name] = proxyCallErrorCount.WithLabelValues(p.Protocol(), "rpc", name)
	}
	h.summary = summary
	h.histogram = histogram
	h.errors = errors
	return h
}

// RPCHandlerFunc ...
type RPCHandlerFunc func(Client, centrifuge.RPCEvent, *config.Container, PerCallData) (centrifuge.RPCReply, error)

// Handle RPC.
func (h *RPCHandler) Handle(node *centrifuge.Node) RPCHandlerFunc {
	return func(client Client, e centrifuge.RPCEvent, cfgContainer *config.Container, pcd PerCallData) (centrifuge.RPCReply, error) {
		started := time.Now()

		var p RPCProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		rpcOpts, ok, err := cfgContainer.RpcOptions(e.Method)
		if err != nil {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error getting RPC options", map[string]any{"method": e.Method, "error": err.Error()}))
			return centrifuge.RPCReply{}, centrifuge.ErrorInternal
		}
		if !ok {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "rpc options not found", map[string]any{"method": e.Method}))
			return centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound
		}
		proxyName := rpcOpts.RpcProxyName
		if proxyName == "" {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "rpc proxy not configured for a method", map[string]any{"method": e.Method}))
			return centrifuge.RPCReply{}, centrifuge.ErrorNotAvailable
		}
		p = h.config.Proxies[proxyName]
		summary = h.summary[proxyName]
		histogram = h.histogram[proxyName]
		errors = h.errors[proxyName]

		req := &proxyproto.RPCRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(p.UseBase64()),

			User:   client.UserID(),
			Method: e.Method,
		}
		if p.IncludeMeta() && pcd.Meta != nil {
			req.Meta = proxyproto.Raw(pcd.Meta)
		}
		if !p.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}

		rpcRep, err := p.ProxyRPC(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.RPCReply{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying RPC", map[string]any{"error": err.Error()}))
			return centrifuge.RPCReply{}, centrifuge.ErrorInternal
		}
		summary.Observe(duration)
		histogram.Observe(duration)
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
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]any{"client": client.ID(), "error": err.Error()}))
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
