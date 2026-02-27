package proxy

import (
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/metrics"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// MapRemoveHandlerConfig ...
type MapRemoveHandlerConfig struct {
	Proxies map[string]MapRemoveProxy
}

// MapRemoveHandler ...
type MapRemoveHandler struct {
	config    MapRemoveHandlerConfig
	summary   map[string]prometheus.Observer
	histogram map[string]prometheus.Observer
	errors    map[string]prometheus.Counter
	inflight  map[string]prometheus.Gauge
}

// NewMapRemoveHandler ...
func NewMapRemoveHandler(c MapRemoveHandlerConfig) *MapRemoveHandler {
	h := &MapRemoveHandler{
		config: c,
	}
	summary := map[string]prometheus.Observer{}
	histogram := map[string]prometheus.Observer{}
	errors := map[string]prometheus.Counter{}
	inflight := map[string]prometheus.Gauge{}
	for name, p := range c.Proxies {
		summary[name] = metrics.ProxyCallDurationSummary.WithLabelValues(p.Protocol(), "map_remove", name)
		histogram[name] = metrics.ProxyCallDurationHistogram.WithLabelValues(p.Protocol(), "map_remove", name)
		errors[name] = metrics.ProxyCallErrorCount.WithLabelValues(p.Protocol(), "map_remove", name)
		inflight[name] = metrics.ProxyCallInflightRequests.WithLabelValues(p.Protocol(), "map_remove", name)
	}
	h.summary = summary
	h.histogram = histogram
	h.errors = errors
	h.inflight = inflight
	return h
}

// MapRemoveHandlerFunc ...
type MapRemoveHandlerFunc func(Client, centrifuge.MapRemoveEvent, configtypes.ChannelOptions, PerCallData) (centrifuge.MapRemoveReply, error)

// Handle MapRemove.
func (h *MapRemoveHandler) Handle(node *centrifuge.Node) MapRemoveHandlerFunc {
	return func(client Client, e centrifuge.MapRemoveEvent, chOpts configtypes.ChannelOptions, pcd PerCallData) (centrifuge.MapRemoveReply, error) {
		started := time.Now()

		var p MapRemoveProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		proxyEnabled := chOpts.MapRemoveProxyEnabled
		proxyName := chOpts.MapRemoveProxyName
		if !proxyEnabled {
			log.Info().Str("channel", e.Channel).Msg("map remove proxy not enabled for a channel")
			return centrifuge.MapRemoveReply{}, centrifuge.ErrorNotAvailable
		}
		p = h.config.Proxies[proxyName]
		summary = h.summary[proxyName]
		histogram = h.histogram[proxyName]
		errors = h.errors[proxyName]
		inflight := h.inflight[proxyName]
		inflight.Inc()
		defer inflight.Dec()

		req := &proxyproto.MapRemoveRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(p.UseBase64()),

			User:    client.UserID(),
			Channel: e.Channel,
			Key:     e.Key,
		}
		if p.IncludeMeta() && pcd.Meta != nil {
			req.Meta = proxyproto.Raw(pcd.Meta)
		}

		mapRemoveRep, err := p.ProxyMapRemove(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				return centrifuge.MapRemoveReply{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			log.Error().Err(err).Str("client", client.ID()).Str("channel", e.Channel).Msg("error proxying map remove")
			return centrifuge.MapRemoveReply{}, err
		}
		summary.Observe(duration)
		histogram.Observe(duration)

		if mapRemoveRep.Disconnect != nil {
			return centrifuge.MapRemoveReply{}, proxyproto.DisconnectFromProto(mapRemoveRep.Disconnect)
		}
		if mapRemoveRep.Error != nil {
			return centrifuge.MapRemoveReply{}, proxyproto.ErrorFromProto(mapRemoveRep.Error)
		}

		key := e.Key
		if mapRemoveRep.Result != nil {
			if mapRemoveRep.Result.Key != "" {
				key = mapRemoveRep.Result.Key
			}
		}

		result, err := node.MapRemove(
			client.Context(), e.Channel, key,
			centrifuge.MapRemoveOptions{},
		)
		return centrifuge.MapRemoveReply{Result: &result}, err
	}
}
