package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// ConnectHandlerConfig ...
type ConnectHandlerConfig struct {
	Proxy ConnectProxy
}

// ConnectHandler ...
type ConnectHandler struct {
	config        ConnectHandlerConfig
	ruleContainer *rule.Container
	summary       prometheus.Observer
	histogram     prometheus.Observer
	errors        prometheus.Counter
}

// NewConnectHandler ...
func NewConnectHandler(c ConnectHandlerConfig, ruleContainer *rule.Container) *ConnectHandler {
	return &ConnectHandler{
		config:        c,
		ruleContainer: ruleContainer,
		summary:       proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "connect"),
		histogram:     proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "connect"),
		errors:        proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "connect"),
	}
}

// Handle returns connecting handler func.
func (h *ConnectHandler) Handle(node *centrifuge.Node) centrifuge.ConnectingHandler {
	return func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		started := time.Now()
		req := &proxyproto.ConnectRequest{
			Client:    e.ClientID,
			Protocol:  string(e.Transport.Protocol()),
			Transport: e.Transport.Name(),
			Encoding:  getEncoding(h.config.Proxy.UseBase64()),
			Name:      e.Name,
			Version:   e.Version,
			Channels:  e.Channels,
		}
		if !h.config.Proxy.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}

		connectRep, err := h.config.Proxy.ProxyConnect(ctx, req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-ctx.Done():
				// Client connection already closed.
				return centrifuge.ConnectReply{}, centrifuge.DisconnectNormal
			default:
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
			return centrifuge.ConnectReply{}, proxyproto.DisconnectFromProto(connectRep.Disconnect)
		}
		if connectRep.Error != nil {
			return centrifuge.ConnectReply{}, proxyproto.ErrorFromProto(connectRep.Error)
		}

		result := connectRep.Result
		if result == nil {
			return centrifuge.ConnectReply{Credentials: nil}, nil
		}

		var info []byte
		if result.B64Info != "" {
			decodedInfo, err := base64.StdEncoding.DecodeString(result.B64Info)
			if err != nil {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
				return centrifuge.ConnectReply{}, centrifuge.ErrorInternal
			}
			info = decodedInfo
		} else {
			info = result.Info
		}

		var data []byte
		if result.B64Data != "" {
			decodedData, err := base64.StdEncoding.DecodeString(result.B64Data)
			if err != nil {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
				return centrifuge.ConnectReply{}, centrifuge.ErrorInternal
			}
			data = decodedData
		} else {
			data = result.Data
		}

		reply := centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   result.User,
				ExpireAt: result.ExpireAt,
				Info:     info,
			},
		}
		if len(data) > 0 {
			reply.Data = data
		}
		if len(result.Channels) > 0 {
			subscriptions := make(map[string]centrifuge.SubscribeOptions, len(result.Channels))
			for _, ch := range result.Channels {
				_, chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
				if err != nil {
					return centrifuge.ConnectReply{}, err
				}
				if !found {
					return centrifuge.ConnectReply{}, centrifuge.ErrorUnknownChannel
				}
				subscriptions[ch] = centrifuge.SubscribeOptions{
					Presence:  chOpts.Presence,
					JoinLeave: chOpts.JoinLeave,
					Recover:   chOpts.Recover,
				}
			}
			reply.Subscriptions = subscriptions
		}
		if result.Meta != nil {
			newCtx := clientcontext.SetContextConnectionMeta(ctx, clientcontext.ConnectionMeta{
				Meta: json.RawMessage(result.Meta),
			})
			reply.Context = newCtx
		}

		return reply, nil
	}
}
