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
type SubscribeHandlerFunc func(*centrifuge.Client, centrifuge.SubscribeEvent, rule.NamespaceChannelOptions) (centrifuge.SubscribeReply, error)

// Handle Subscribe.
func (h *SubscribeHandler) Handle(node *centrifuge.Node) SubscribeHandlerFunc {
	return func(client *centrifuge.Client, e centrifuge.SubscribeEvent, chOpts rule.NamespaceChannelOptions) (centrifuge.SubscribeReply, error) {
		started := time.Now()
		subscribeRep, err := h.config.Proxy.ProxySubscribe(client.Context(), SubscribeRequest{
			ClientID:  client.ID(),
			UserID:    client.UserID(),
			Channel:   e.Channel,
			Token:     e.Token,
			Transport: client.Transport(),
		})
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.SubscribeReply{}, nil
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
			return centrifuge.SubscribeReply{}, subscribeRep.Disconnect
		}
		if subscribeRep.Error != nil {
			return centrifuge.SubscribeReply{}, subscribeRep.Error
		}

		var info []byte
		if subscribeRep.Result != nil {
			if client.Transport().Encoding() == "json" {
				info = subscribeRep.Result.Info
			} else {
				if subscribeRep.Result.Base64Info != "" {
					decodedInfo, err := base64.StdEncoding.DecodeString(subscribeRep.Result.Base64Info)
					if err != nil {
						node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
						return centrifuge.SubscribeReply{}, centrifuge.ErrorInternal
					}
					info = decodedInfo
				}
			}
		}

		return centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ChannelInfo: info,
				Presence:    chOpts.Presence,
				JoinLeave:   chOpts.JoinLeave,
				Recover:     chOpts.HistoryRecover,
			},
			ClientSideRefresh: true,
		}, nil
	}
}
