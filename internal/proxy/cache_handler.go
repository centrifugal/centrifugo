package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/prometheus/client_golang/prometheus"
)

// CacheHandlerConfig ...
type CacheHandlerConfig struct {
	Proxies map[string]CacheEmptyProxy
}

// CacheHandler ...
type CacheHandler struct {
	config    CacheHandlerConfig
	summary   map[string]prometheus.Observer
	histogram map[string]prometheus.Observer
	errors    map[string]prometheus.Counter
}

// NewCacheHandler ...
func NewCacheHandler(c CacheHandlerConfig) *CacheHandler {
	h := &CacheHandler{
		config: c,
	}
	summary := map[string]prometheus.Observer{}
	histogram := map[string]prometheus.Observer{}
	errors := map[string]prometheus.Counter{}
	for name, p := range c.Proxies {
		summary[name] = proxyCallDurationSummary.WithLabelValues(p.Protocol(), "cache_empty", name)
		histogram[name] = proxyCallDurationHistogram.WithLabelValues(p.Protocol(), "cache_empty", name)
		errors[name] = proxyCallErrorCount.WithLabelValues(p.Protocol(), "cache_empty", name)
	}
	h.summary = summary
	h.histogram = histogram
	h.errors = errors
	return h
}

// CacheEmptyHandlerFunc ...
type CacheEmptyHandlerFunc func(
	ctx context.Context, channel string, chOpts configtypes.ChannelOptions) (centrifuge.CacheEmptyReply, error)

// Handle Document.
func (h *CacheHandler) Handle(node *centrifuge.Node) CacheEmptyHandlerFunc {
	return func(ctx context.Context, channel string, chOpts configtypes.ChannelOptions) (centrifuge.CacheEmptyReply, error) {
		started := time.Now()

		var p CacheEmptyProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		proxyName := chOpts.CacheEmptyProxyName
		if proxyName == "" {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "cache empty proxy not configured for a channel", map[string]any{"channel": channel}))
			return centrifuge.CacheEmptyReply{}, centrifuge.ErrorNotAvailable
		}
		p = h.config.Proxies[proxyName]
		summary = h.summary[proxyName]
		histogram = h.histogram[proxyName]
		errors = h.errors[proxyName]

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
