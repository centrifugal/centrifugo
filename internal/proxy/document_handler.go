package proxy

import (
	"context"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// DocumentHandlerConfig ...
type DocumentHandlerConfig struct {
	Proxies           map[string]DocumentProxy
	GranularProxyMode bool
}

// DocumentHandler ...
type DocumentHandler struct {
	config            DocumentHandlerConfig
	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewDocumentHandler ...
func NewDocumentHandler(c DocumentHandlerConfig) *DocumentHandler {
	h := &DocumentHandler{
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

type DocumentExtra struct {
}

// DocumentHandlerFunc ...
type DocumentHandlerFunc func(ctx context.Context, id string, chOpts rule.ChannelOptions) (*proxyproto.Document, error)

// Handle Document.
func (h *DocumentHandler) Handle(node *centrifuge.Node) DocumentHandlerFunc {
	return func(ctx context.Context, id string, chOpts rule.ChannelOptions) (*proxyproto.Document, error) {
		started := time.Now()

		var p DocumentProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		if h.config.GranularProxyMode {
			//proxyName := chOpts.DocumentProxyName
			//if proxyName == "" {
			//	node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "subscribe proxy not configured for a channel", map[string]any{"channel": e.Channel}))
			//	return centrifuge.DocumentReply{}, DocumentExtra{}, centrifuge.ErrorNotAvailable
			//}
			//p = h.config.Proxies[proxyName]
			//summary = h.granularSummary[proxyName]
			//histogram = h.granularHistogram[proxyName]
			//errors = h.granularErrors[proxyName]
		} else {
			p = h.config.Proxies[""]
			summary = h.summary
			histogram = h.histogram
			errors = h.errors
		}

		req := &proxyproto.LoadDocumentsRequest{
			Ids: []string{id},
		}
		resp, err := p.LoadDocuments(ctx, req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-ctx.Done():
				// Client connection already closed.
				return nil, ctx.Err()
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error loading documents", map[string]any{"error": err.Error()}))
			return nil, centrifuge.ErrorInternal
		}
		summary.Observe(duration)
		histogram.Observe(duration)
		// TODO: handle missing id in result.
		return resp.Result.Documents[id], nil
	}
}
