package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v5/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// SubRefreshHandlerConfig ...
type SubRefreshHandlerConfig struct {
	Proxies           map[string]SubRefreshProxy
	GranularProxyMode bool
}

// SubRefreshHandler ...
type SubRefreshHandler struct {
	config            SubRefreshHandlerConfig
	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewSubRefreshHandler ...
func NewSubRefreshHandler(c SubRefreshHandlerConfig) *SubRefreshHandler {
	h := &SubRefreshHandler{
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
			summary[name] = granularProxyCallDurationSummary.WithLabelValues("sub_refresh", name)
			histogram[name] = granularProxyCallDurationHistogram.WithLabelValues("sub_refresh", name)
			errors[name] = granularProxyCallErrorCount.WithLabelValues("sub_refresh", name)
		}
		h.granularSummary = summary
		h.granularHistogram = histogram
		h.granularErrors = errors
	} else {
		h.summary = proxyCallDurationSummary.WithLabelValues(h.config.Proxies[""].Protocol(), "sub_refresh")
		h.histogram = proxyCallDurationHistogram.WithLabelValues(h.config.Proxies[""].Protocol(), "sub_refresh")
		h.errors = proxyCallErrorCount.WithLabelValues(h.config.Proxies[""].Protocol(), "sub_refresh")
	}
	return h
}

type SubRefreshExtra struct {
}

// SubRefreshHandlerFunc ...
type SubRefreshHandlerFunc func(Client, centrifuge.SubRefreshEvent, rule.ChannelOptions, PerCallData) (centrifuge.SubRefreshReply, SubRefreshExtra, error)

// Handle refresh.
func (h *SubRefreshHandler) Handle(node *centrifuge.Node) SubRefreshHandlerFunc {
	return func(client Client, e centrifuge.SubRefreshEvent, chOpts rule.ChannelOptions, pcd PerCallData) (centrifuge.SubRefreshReply, SubRefreshExtra, error) {
		started := time.Now()

		var p SubRefreshProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		if h.config.GranularProxyMode {
			proxyName := chOpts.SubRefreshProxyName
			if proxyName == "" {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "sub refresh proxy not configured for a channel", map[string]any{"channel": e.Channel}))
				return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.ErrorNotAvailable
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

		req := &proxyproto.SubRefreshRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(p.UseBase64()),

			User:    client.UserID(),
			Channel: e.Channel,
		}
		if p.IncludeMeta() && pcd.Meta != nil {
			req.Meta = proxyproto.Raw(pcd.Meta)
		}
		refreshRep, err := p.ProxySubRefresh(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying sub refresh", map[string]any{"error": err.Error()}))
			// In case of an error give connection one more minute to live and
			// then try to check again. This way we gracefully handle temporary
			// problems on application backend side.
			// NOTE: this interval must be configurable maybe, but for now looks
			// like a reasonable value.
			return centrifuge.SubRefreshReply{
				ExpireAt: time.Now().Unix() + 60,
			}, SubRefreshExtra{}, nil
		}
		summary.Observe(duration)
		histogram.Observe(duration)

		result := refreshRep.Result
		if result == nil {
			// Subscription will be unsubscribed.
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "no sub refresh result found", map[string]any{}))
			return centrifuge.SubRefreshReply{
				Expired: true,
			}, SubRefreshExtra{}, nil
		}

		if result.Expired {
			return centrifuge.SubRefreshReply{
				Expired: true,
			}, SubRefreshExtra{}, nil
		}

		var info []byte
		if result.B64Info != "" {
			decodedInfo, err := base64.StdEncoding.DecodeString(result.B64Info)
			if err != nil {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]any{"client": client.ID(), "error": err.Error()}))
				return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.ErrorInternal
			}
			info = decodedInfo
		} else {
			info = result.Info
		}

		extra := SubRefreshExtra{}

		return centrifuge.SubRefreshReply{
			ExpireAt: result.ExpireAt,
			Info:     info,
		}, extra, nil
	}
}
