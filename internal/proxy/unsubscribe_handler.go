package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/prometheus/client_golang/prometheus"
)

// UnsubscribeHandlerConfig ...
type UnsubscribeHandlerConfig struct {
	Proxies           map[string]UnsubscribeProxy
	GranularProxyMode bool
}

// UnsubscribeHandler ...
type UnsubscribeHandler struct {
	config            UnsubscribeHandlerConfig
	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewUnsubscribeHandler ...
func NewUnsubscribeHandler(c UnsubscribeHandlerConfig) *UnsubscribeHandler {
	h := &UnsubscribeHandler{
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
			summary[name] = granularProxyCallDurationSummary.WithLabelValues("unsubscribe", name)
			histogram[name] = granularProxyCallDurationHistogram.WithLabelValues("unsubscribe", name)
			errors[name] = granularProxyCallErrorCount.WithLabelValues("unsubscribe", name)
		}
		h.granularSummary = summary
		h.granularHistogram = histogram
		h.granularErrors = errors
	} else {
		h.summary = proxyCallDurationSummary.WithLabelValues(h.config.Proxies[""].Protocol(), "unsubscribe")
		h.histogram = proxyCallDurationHistogram.WithLabelValues(h.config.Proxies[""].Protocol(), "unsubscribe")
		h.errors = proxyCallErrorCount.WithLabelValues(h.config.Proxies[""].Protocol(), "unsubscribe")
	}
	return h
}

// UnsubscribeHandlerFunc ...
type UnsubscribeHandlerFunc func(Client, centrifuge.UnsubscribeEvent, rule.ChannelOptions, PerCallData)

// Handle Unsubscribe.
func (h *UnsubscribeHandler) Handle(node *centrifuge.Node) UnsubscribeHandlerFunc {
	return func(client Client, e centrifuge.UnsubscribeEvent, chOpts rule.ChannelOptions, pcd PerCallData) {
		started := time.Now()

		var p UnsubscribeProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		if h.config.GranularProxyMode {
			proxyName := chOpts.UnsubscribeProxyName
			if proxyName == "" {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "unsubscribe proxy not configured for a channel", map[string]any{"channel": e.Channel}))
				return
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

		req := &proxyproto.UnsubscribeRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(p.UseBase64()),

			User:    client.UserID(),
			Channel: e.Channel,
		}
		_, err := p.ProxyUnsubscribe(context.Background(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying unsubscribe", map[string]any{"error": err.Error()}))
			return
		}
		summary.Observe(duration)
		histogram.Observe(duration)
	}
}
