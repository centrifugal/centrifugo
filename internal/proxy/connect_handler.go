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
	config  ConnectHandlerConfig
	summary prometheus.Observer
	errors  prometheus.Counter
}

// NewConnectHandler ...
func NewConnectHandler(c ConnectHandlerConfig) *ConnectHandler {
	return &ConnectHandler{
		config:  c,
		summary: proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "connect"),
		errors:  proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "connect"),
	}
}

// Handle returns connecting handler func.
func (h *ConnectHandler) Handle(node *centrifuge.Node) func(ctx context.Context, t centrifuge.TransportInfo, e centrifuge.ConnectEvent) centrifuge.ConnectReply {
	return func(ctx context.Context, t centrifuge.TransportInfo, e centrifuge.ConnectEvent) centrifuge.ConnectReply {

		if e.Token != "" {
			// As soon as token provided we do not proxy connect to application backend.
			return centrifuge.ConnectReply{
				Credentials: nil,
			}
		}

		started := time.Now()
		connectRep, err := h.config.Proxy.ProxyConnect(ctx, ConnectRequest{
			ClientID:  e.ClientID,
			Transport: t,
			Data:      e.Data,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.ConnectReply{
					Disconnect: centrifuge.DisconnectNormal,
				}
			}
			h.summary.Observe(time.Since(started).Seconds())
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying connect", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
			return centrifuge.ConnectReply{
				Error: centrifuge.ErrorInternal,
			}
		}
		h.summary.Observe(time.Since(started).Seconds())
		if connectRep.Disconnect != nil {
			return centrifuge.ConnectReply{
				Disconnect: connectRep.Disconnect,
			}
		}
		if connectRep.Error != nil {
			return centrifuge.ConnectReply{
				Error: connectRep.Error,
			}
		}

		credentials := connectRep.Result
		if credentials == nil {
			return centrifuge.ConnectReply{
				Credentials: nil,
			}
		}

		var info []byte
		if t.Encoding() == "json" {
			info = credentials.Info
		} else {
			if credentials.Base64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(credentials.Base64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
					return centrifuge.ConnectReply{
						Error: centrifuge.ErrorInternal,
					}
				}
				info = decodedInfo
			}
		}

		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   credentials.UserID,
				ExpireAt: credentials.ExpireAt,
				Info:     info,
			},
		}
	}
}
