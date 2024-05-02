package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// CacheHandlerConfig ...
type CacheHandlerConfig struct {
	Proxies           map[string]CacheEmptyProxy
	GranularProxyMode bool
}

// CacheHandler ...
type CacheHandler struct {
	config            CacheHandlerConfig
	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewCacheHandler ...
func NewCacheHandler(c CacheHandlerConfig) *CacheHandler {
	h := &CacheHandler{
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

// CacheEmptyHandlerFunc ...
type CacheEmptyHandlerFunc func(ctx context.Context, channel string, chOpts rule.ChannelOptions) (centrifuge.CacheEmptyReply, error)

// Handle Document.
func (h *CacheHandler) Handle(node *centrifuge.Node) CacheEmptyHandlerFunc {
	return func(ctx context.Context, channel string, chOpts rule.ChannelOptions) (centrifuge.CacheEmptyReply, error) {
		started := time.Now()

		var p CacheEmptyProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		if h.config.GranularProxyMode {
			proxyName := chOpts.CacheEmptyProxyName
			if proxyName == "" {
				node.Log(centrifuge.NewLogEntry(
					centrifuge.LogLevelInfo,
					"cache proxy not configured for a channel",
					map[string]any{"channel": channel}))
				return centrifuge.CacheEmptyReply{}, centrifuge.ErrorNotAvailable
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

		req := &proxyproto.NotifyCacheEmptyRequest{
			Channel: channel,
		}
		resp, err := p.NotifyCacheEmpty(ctx, req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-ctx.Done():
				// Client connection already closed.
				return centrifuge.CacheEmptyReply{}, ctx.Err()
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error notifying about empty cache", map[string]any{"channel": channel, "error": err.Error()}))
			return centrifuge.CacheEmptyReply{}, centrifuge.ErrorInternal
		}
		summary.Observe(duration)
		histogram.Observe(duration)
		return centrifuge.CacheEmptyReply{
			Populated: resp.Result.Populated,
		}, nil
	}
}
