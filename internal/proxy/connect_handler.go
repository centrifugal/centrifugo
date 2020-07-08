package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// ConnectHandlerConfig ...
type ConnectHandlerConfig struct {
	Proxy ConnectProxy
}

// ConnectHandler ...
type ConnectHandler struct {
	config    ConnectHandlerConfig
	summary   prometheus.Observer
	histogram prometheus.Observer
	errors    prometheus.Counter
}

// NewConnectHandler ...
func NewConnectHandler(c ConnectHandlerConfig) *ConnectHandler {
	return &ConnectHandler{
		config:    c,
		summary:   proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "connect"),
		histogram: proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "connect"),
		errors:    proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "connect"),
	}
}

// Handle returns connecting handler func.
func (h *ConnectHandler) Handle(node *centrifuge.Node) func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
	return func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		started := time.Now()
		connectRep, err := h.config.Proxy.ProxyConnect(ctx, ConnectRequest{
			ClientID:  e.ClientID,
			Transport: e.Transport,
			Data:      e.Data,
		})
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.ConnectReply{}, centrifuge.DisconnectNormal
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying connect", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
			return centrifuge.ConnectReply{}, centrifuge.ErrorInternal
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)
		if connectRep.Disconnect != nil {
			return centrifuge.ConnectReply{}, connectRep.Disconnect
		}
		if connectRep.Error != nil {
			return centrifuge.ConnectReply{}, connectRep.Error
		}

		credentials := connectRep.Result
		if credentials == nil {
			return centrifuge.ConnectReply{Credentials: nil}, nil
		}

		var info []byte
		if e.Transport.Encoding() == "json" {
			info = credentials.Info
		} else {
			if credentials.Base64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(credentials.Base64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
					return centrifuge.ConnectReply{}, centrifuge.ErrorInternal
				}
				info = decodedInfo
			}
		}

		var data []byte
		if e.Transport.Encoding() == "json" {
			data = credentials.Data
		} else {
			if credentials.Base64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(credentials.Base64Data)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
					return centrifuge.ConnectReply{}, centrifuge.ErrorInternal
				}
				data = decodedData
			}
		}

		reply := centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   credentials.UserID,
				ExpireAt: credentials.ExpireAt,
				Info:     info,
			},
			Channels: credentials.Channels,
		}
		if len(data) > 0 {
			reply.Data = data
		}
		return reply, nil
	}
}
