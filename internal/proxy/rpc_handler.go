package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v4/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// RPCHandlerConfig ...
type RPCHandlerConfig struct {
	Proxies           map[string]RPCProxy
	GranularProxyMode bool
}

// RPCHandler ...
type RPCHandler struct {
	config            RPCHandlerConfig
	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewRPCHandler ...
func NewRPCHandler(c RPCHandlerConfig) *RPCHandler {
	h := &RPCHandler{
		config: c,
	}
	if h.config.GranularProxyMode {
		summary := map[string]prometheus.Observer{}
		histogram := map[string]prometheus.Observer{}
		errors := map[string]prometheus.Counter{}
		for k := range c.Proxies {
			name := k
			if name == "" {
				name = "__default__"
			}
			summary[name] = granularProxyCallDurationSummary.WithLabelValues("rpc", name)
			histogram[name] = granularProxyCallDurationHistogram.WithLabelValues("rpc", name)
			errors[name] = granularProxyCallErrorCount.WithLabelValues("rpc", name)
		}
		h.granularSummary = summary
		h.granularHistogram = histogram
		h.granularErrors = errors
	} else {
		h.summary = proxyCallDurationSummary.WithLabelValues(h.config.Proxies[""].Protocol(), "rpc")
		h.histogram = proxyCallDurationHistogram.WithLabelValues(h.config.Proxies[""].Protocol(), "rpc")
		h.errors = proxyCallErrorCount.WithLabelValues(h.config.Proxies[""].Protocol(), "rpc")
	}
	return h
}

// RPCHandlerFunc ...
type RPCHandlerFunc func(*centrifuge.Client, centrifuge.RPCEvent, *rule.Container, PerCallData) (centrifuge.RPCReply, error)

// Handle RPC.
func (h *RPCHandler) Handle(node *centrifuge.Node) RPCHandlerFunc {
	return func(client *centrifuge.Client, e centrifuge.RPCEvent, ruleContainer *rule.Container, pcd PerCallData) (centrifuge.RPCReply, error) {
		started := time.Now()

		var p RPCProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		if h.config.GranularProxyMode {
			rpcOpts, ok, err := ruleContainer.RpcOptions(e.Method)
			if err != nil {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error getting RPC options", map[string]interface{}{"method": e.Method, "error": err.Error()}))
				return centrifuge.RPCReply{}, centrifuge.ErrorInternal
			}
			if !ok {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "rpc options not found", map[string]interface{}{"method": e.Method}))
				return centrifuge.RPCReply{}, centrifuge.ErrorMethodNotFound
			}
			proxyName := rpcOpts.RpcProxyName
			if proxyName == "" {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "rpc proxy not configured for a method", map[string]interface{}{"method": e.Method}))
				return centrifuge.RPCReply{}, centrifuge.ErrorNotAvailable
			}
			p = h.config.Proxies[proxyName]
			summary = h.granularSummary[proxyName]
			histogram = h.granularHistogram[proxyName]
			errors = h.granularErrors[proxyName]
		} else {
			p = h.config.Proxies[""]
			summary = h.summary
			histogram = h.histogram
			errors = h.errors
		}

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
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying RPC", map[string]interface{}{"error": err.Error()}))
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
