package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// SubscribeHandlerConfig ...
type SubscribeHandlerConfig struct {
	Proxy SubscribeProxy
}

// SubscribeHandler ...
type SubscribeHandler struct {
	config    SubscribeHandlerConfig
	summary   prometheus.Observer
	histogram prometheus.Observer
	errors    prometheus.Counter
}

// NewSubscribeHandler ...
func NewSubscribeHandler(c SubscribeHandlerConfig) *SubscribeHandler {
	return &SubscribeHandler{
		config:    c,
		summary:   proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "subscribe"),
		histogram: proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "subscribe"),
		errors:    proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "subscribe"),
	}
}

// SubscribeHandlerFunc ...
type SubscribeHandlerFunc func(*centrifuge.Client, centrifuge.SubscribeEvent, rule.ChannelOptions) (centrifuge.SubscribeReply, error)

// Handle Subscribe.
func (h *SubscribeHandler) Handle(node *centrifuge.Node) SubscribeHandlerFunc {
	return func(client *centrifuge.Client, e centrifuge.SubscribeEvent, chOpts rule.ChannelOptions) (centrifuge.SubscribeReply, error) {
		started := time.Now()

		req := &proxyproto.SubscribeRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(h.config.Proxy.UseBase64()),

			User:    client.UserID(),
			Channel: e.Channel,
			Token:   e.Token,
		}
		if h.config.Proxy.IncludeMeta() {
			if connMeta, ok := clientcontext.GetContextConnectionMeta(client.Context()); ok {
				req.Meta = proxyproto.Raw(connMeta.Meta)
			}
		}
		subscribeRep, err := h.config.Proxy.ProxySubscribe(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.SubscribeReply{}, centrifuge.DisconnectNormal
			default:
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying subscribe", map[string]interface{}{"error": err.Error()}))
			return centrifuge.SubscribeReply{}, centrifuge.ErrorInternal
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)

		if subscribeRep.Disconnect != nil {
			return centrifuge.SubscribeReply{}, proxyproto.DisconnectFromProto(subscribeRep.Disconnect)
		}
		if subscribeRep.Error != nil {
			return centrifuge.SubscribeReply{}, proxyproto.ErrorFromProto(subscribeRep.Error)
		}

		presence := chOpts.Presence
		joinLeave := chOpts.JoinLeave
		useRecover := chOpts.Recover
		position := chOpts.Position

		var info []byte
		var data []byte
		if subscribeRep.Result != nil {
			if subscribeRep.Result.B64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(subscribeRep.Result.B64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.SubscribeReply{}, centrifuge.ErrorInternal
				}
				info = decodedInfo
			} else {
				info = subscribeRep.Result.Info
			}
			if subscribeRep.Result.B64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(subscribeRep.Result.B64Data)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.SubscribeReply{}, centrifuge.ErrorInternal
				}
				data = decodedData
			} else {
				data = subscribeRep.Result.Data
			}

			result := subscribeRep.Result

			if result.Override != nil && result.Override.Presence != nil {
				presence = result.Override.Presence.Value
			}
			if result.Override != nil && result.Override.JoinLeave != nil {
				joinLeave = result.Override.JoinLeave.Value
			}
			if result.Override != nil && result.Override.Recover != nil {
				useRecover = result.Override.Recover.Value
			}
			if result.Override != nil && result.Override.Position != nil {
				position = result.Override.Position.Value
			}
		}

		return centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ChannelInfo: info,
				Presence:    presence,
				JoinLeave:   joinLeave,
				Recover:     useRecover,
				Position:    position,
				Data:        data,
			},
			ClientSideRefresh: true,
		}, nil
	}
}
