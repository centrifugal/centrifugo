package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/centrifugal/centrifugo/internal/rule"

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
	ruleContainer *rule.ChannelRuleContainer
	summary       prometheus.Observer
	histogram     prometheus.Observer
	errors        prometheus.Counter
}

// NewConnectHandler ...
func NewConnectHandler(c ConnectHandlerConfig, ruleContainer *rule.ChannelRuleContainer) *ConnectHandler {
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
	return func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		started := time.Now()
		connectRep, err := h.config.Proxy.ProxyConnect(ctx, ConnectRequest{
			ClientID:  e.ClientID,
			Transport: e.Transport,
			Data:      e.Data,
			Name:      e.Name,
			Version:   e.Version,
		})
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.ConnectResult{}, centrifuge.DisconnectNormal
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying connect", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
			return centrifuge.ConnectResult{}, centrifuge.ErrorInternal
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)
		if connectRep.Disconnect != nil {
			return centrifuge.ConnectResult{}, connectRep.Disconnect
		}
		if connectRep.Error != nil {
			return centrifuge.ConnectResult{}, connectRep.Error
		}

		credentials := connectRep.Result
		if credentials == nil {
			return centrifuge.ConnectResult{Credentials: nil}, nil
		}

		var info []byte
		if e.Transport.Encoding() == "json" {
			info = credentials.Info
		} else {
			if credentials.Base64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(credentials.Base64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": e.ClientID, "error": err.Error()}))
					return centrifuge.ConnectResult{}, centrifuge.ErrorInternal
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
					return centrifuge.ConnectResult{}, centrifuge.ErrorInternal
				}
				data = decodedData
			}
		}

		reply := centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID:   credentials.UserID,
				ExpireAt: credentials.ExpireAt,
				Info:     info,
			},
		}
		if len(data) > 0 {
			reply.Data = data
		}
		if len(credentials.Channels) > 0 {
			subscriptions := make([]centrifuge.Subscription, 0, len(credentials.Channels))
			for _, ch := range credentials.Channels {
				chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
				if err != nil {
					return centrifuge.ConnectResult{}, err
				}
				if !found {
					return centrifuge.ConnectResult{}, centrifuge.ErrorUnknownChannel
				}
				subscriptions = append(subscriptions, centrifuge.Subscription{
					Channel:   ch,
					Presence:  chOpts.Presence,
					JoinLeave: chOpts.JoinLeave,
					Recover:   chOpts.HistoryRecover,
				})
			}
			reply.Subscriptions = subscriptions
		}
		return reply, nil
	}
}
