package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// SubRefreshHandlerConfig ...
type SubRefreshHandlerConfig struct {
	Proxies map[string]SubRefreshProxy
}

// SubRefreshHandler ...
type SubRefreshHandler struct {
	config    SubRefreshHandlerConfig
	summary   map[string]prometheus.Observer
	histogram map[string]prometheus.Observer
	errors    map[string]prometheus.Counter
	inflight  map[string]prometheus.Gauge
}

// NewSubRefreshHandler ...
func NewSubRefreshHandler(c SubRefreshHandlerConfig) *SubRefreshHandler {
	h := &SubRefreshHandler{
		config: c,
	}
	summary := map[string]prometheus.Observer{}
	histogram := map[string]prometheus.Observer{}
	errors := map[string]prometheus.Counter{}
	inflight := map[string]prometheus.Gauge{}
	for name, p := range c.Proxies {
		summary[name] = proxyCallDurationSummary.WithLabelValues(p.Protocol(), "sub_refresh", name)
		histogram[name] = proxyCallDurationHistogram.WithLabelValues(p.Protocol(), "sub_refresh", name)
		errors[name] = proxyCallErrorCount.WithLabelValues(p.Protocol(), "sub_refresh", name)
		inflight[name] = proxyCallInflightRequests.WithLabelValues(p.Protocol(), "sub_refresh", name)
	}
	h.summary = summary
	h.histogram = histogram
	h.errors = errors
	h.inflight = inflight
	return h
}

type SubRefreshExtra struct {
}

// SubRefreshHandlerFunc ...
type SubRefreshHandlerFunc func(Client, centrifuge.SubRefreshEvent, configtypes.ChannelOptions, PerCallData) (centrifuge.SubRefreshReply, SubRefreshExtra, error)

// Handle refresh.
func (h *SubRefreshHandler) Handle() SubRefreshHandlerFunc {
	return func(client Client, e centrifuge.SubRefreshEvent, chOpts configtypes.ChannelOptions, pcd PerCallData) (centrifuge.SubRefreshReply, SubRefreshExtra, error) {
		started := time.Now()

		var p SubRefreshProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		proxyEnabled := chOpts.SubRefreshProxyEnabled
		proxyName := chOpts.SubRefreshProxyName
		if !proxyEnabled {
			log.Info().Str("channel", e.Channel).Msg("sub refresh proxy not configured for a channel")
			return centrifuge.SubRefreshReply{}, SubRefreshExtra{}, centrifuge.ErrorNotAvailable
		}
		p = h.config.Proxies[proxyName]
		summary = h.summary[proxyName]
		histogram = h.histogram[proxyName]
		errors = h.errors[proxyName]
		inflight := h.inflight[proxyName]
		inflight.Inc()
		defer inflight.Dec()

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
			log.Error().Err(err).Str("client", client.ID()).Str("channel", e.Channel).Msg("error proxying sub refresh")
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
			log.Error().Msg("no sub refresh result found")
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
				log.Error().Err(err).Str("client", client.ID()).Msg("error decoding base64 info")
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
