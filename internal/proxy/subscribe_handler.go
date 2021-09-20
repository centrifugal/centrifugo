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
	Proxies           map[string]SubscribeProxy
	GranularProxyMode bool
}

// SubscribeHandler ...
type SubscribeHandler struct {
	config            SubscribeHandlerConfig
	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewSubscribeHandler ...
func NewSubscribeHandler(c SubscribeHandlerConfig) *SubscribeHandler {
	h := &SubscribeHandler{
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
			summary[name] = granularProxyCallDurationSummary.WithLabelValues("subscribe", name)
			histogram[name] = granularProxyCallDurationHistogram.WithLabelValues("subscribe", name)
			errors[name] = granularProxyCallErrorCount.WithLabelValues("subscribe", name)
		}
		h.granularSummary = summary
		h.granularHistogram = histogram
		h.granularErrors = errors
	} else {
		h.summary = proxyCallDurationSummary.WithLabelValues(h.config.Proxies[""].Protocol(), "subscribe")
		h.histogram = proxyCallDurationHistogram.WithLabelValues(h.config.Proxies[""].Protocol(), "subscribe")
		h.errors = proxyCallErrorCount.WithLabelValues(h.config.Proxies[""].Protocol(), "subscribe")
	}
	return h
}

// SubscribeHandlerFunc ...
type SubscribeHandlerFunc func(*centrifuge.Client, centrifuge.SubscribeEvent, rule.ChannelOptions) (centrifuge.SubscribeReply, error)

// Handle Subscribe.
func (h *SubscribeHandler) Handle(node *centrifuge.Node) SubscribeHandlerFunc {
	return func(client *centrifuge.Client, e centrifuge.SubscribeEvent, chOpts rule.ChannelOptions) (centrifuge.SubscribeReply, error) {
		started := time.Now()

		var p SubscribeProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		if h.config.GranularProxyMode {
			proxyName := chOpts.PublishProxyName
			if proxyName == "" {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe proxy not configured for a channel", map[string]interface{}{"channel": e.Channel}))
				return centrifuge.SubscribeReply{}, centrifuge.ErrorNotAvailable
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

		req := &proxyproto.SubscribeRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(p.UseBase64()),

			User:    client.UserID(),
			Channel: e.Channel,
			Token:   e.Token,
		}
		if p.IncludeMeta() {
			if connMeta, ok := clientcontext.GetContextConnectionMeta(client.Context()); ok {
				req.Meta = proxyproto.Raw(connMeta.Meta)
			}
		}
		subscribeRep, err := p.ProxySubscribe(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.SubscribeReply{}, centrifuge.DisconnectNormal
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying subscribe", map[string]interface{}{"error": err.Error()}))
			return centrifuge.SubscribeReply{}, centrifuge.ErrorInternal
		}
		summary.Observe(duration)
		histogram.Observe(duration)

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
			},
			ClientSideRefresh: true,
		}, nil
	}
}
