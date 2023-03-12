package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v4/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v4/internal/rule"
	"github.com/centrifugal/centrifugo/v4/internal/subsource"

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

type ConnectExtra struct {
}

type ConnectingHandlerFunc func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectReply, ConnectExtra, error)

// Handle returns connecting handler func.
func (h *ConnectHandler) Handle(node *centrifuge.Node) ConnectingHandlerFunc {
	return func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, ConnectExtra, error) {
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
				return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying connect", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
			return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorInternal
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)
		if connectRep.Disconnect != nil {
			return centrifuge.ConnectReply{}, ConnectExtra{}, proxyproto.DisconnectFromProto(connectRep.Disconnect)
		}
		if connectRep.Error != nil {
			return centrifuge.ConnectReply{}, ConnectExtra{}, proxyproto.ErrorFromProto(connectRep.Error)
		}

		result := connectRep.Result
		if result == nil {
			return centrifuge.ConnectReply{Credentials: nil}, ConnectExtra{}, nil
		}

		var info []byte
		if result.B64Info != "" {
			decodedInfo, err := base64.StdEncoding.DecodeString(result.B64Info)
			if err != nil {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
				return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorInternal
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
				return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorInternal
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
				_, _, chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
				if err != nil {
					return centrifuge.ConnectReply{}, ConnectExtra{}, err
				}
				if !found {
					return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorUnknownChannel
				}
				subscriptions[ch] = centrifuge.SubscribeOptions{
					EmitPresence:      chOpts.Presence,
					EmitJoinLeave:     chOpts.JoinLeave,
					PushJoinLeave:     chOpts.ForcePushJoinLeave,
					EnableRecovery:    chOpts.ForceRecovery,
					EnablePositioning: chOpts.ForcePositioning,
					Source:            subsource.ConnectProxy,
					HistoryMetaTTL:    time.Duration(chOpts.HistoryMetaTTL),
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

		return reply, ConnectExtra{}, nil
	}
}
