package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/metrics"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// MapPublishHandlerConfig ...
type MapPublishHandlerConfig struct {
	Proxies map[string]MapPublishProxy
}

// MapPublishHandler ...
type MapPublishHandler struct {
	config    MapPublishHandlerConfig
	summary   map[string]prometheus.Observer
	histogram map[string]prometheus.Observer
	errors    map[string]prometheus.Counter
	inflight  map[string]prometheus.Gauge
}

// NewMapPublishHandler ...
func NewMapPublishHandler(c MapPublishHandlerConfig) *MapPublishHandler {
	h := &MapPublishHandler{
		config: c,
	}
	summary := map[string]prometheus.Observer{}
	histogram := map[string]prometheus.Observer{}
	errors := map[string]prometheus.Counter{}
	inflight := map[string]prometheus.Gauge{}
	for name, p := range c.Proxies {
		summary[name] = metrics.ProxyCallDurationSummary.WithLabelValues(p.Protocol(), "map_publish", name)
		histogram[name] = metrics.ProxyCallDurationHistogram.WithLabelValues(p.Protocol(), "map_publish", name)
		errors[name] = metrics.ProxyCallErrorCount.WithLabelValues(p.Protocol(), "map_publish", name)
		inflight[name] = metrics.ProxyCallInflightRequests.WithLabelValues(p.Protocol(), "map_publish", name)
	}
	h.summary = summary
	h.histogram = histogram
	h.errors = errors
	h.inflight = inflight
	return h
}

// MapPublishHandlerFunc ...
type MapPublishHandlerFunc func(Client, centrifuge.MapPublishEvent, configtypes.ChannelOptions, PerCallData) (centrifuge.MapPublishReply, error)

// Handle MapPublish.
func (h *MapPublishHandler) Handle(node *centrifuge.Node) MapPublishHandlerFunc {
	return func(client Client, e centrifuge.MapPublishEvent, chOpts configtypes.ChannelOptions, pcd PerCallData) (centrifuge.MapPublishReply, error) {
		started := time.Now()

		var p MapPublishProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		proxyEnabled := chOpts.MapPublishProxyEnabled
		proxyName := chOpts.MapPublishProxyName
		if !proxyEnabled {
			log.Info().Str("channel", e.Channel).Msg("map publish proxy not enabled for a channel")
			return centrifuge.MapPublishReply{}, centrifuge.ErrorNotAvailable
		}
		p = h.config.Proxies[proxyName]
		summary = h.summary[proxyName]
		histogram = h.histogram[proxyName]
		errors = h.errors[proxyName]
		inflight := h.inflight[proxyName]
		inflight.Inc()
		defer inflight.Dec()

		req := &proxyproto.MapPublishRequest{
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
		if !p.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}

		mapPublishRep, err := p.ProxyMapPublish(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				return centrifuge.MapPublishReply{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			log.Error().Err(err).Str("client", client.ID()).Str("channel", e.Channel).Msg("error proxying map publish")
			return centrifuge.MapPublishReply{}, err
		}
		summary.Observe(duration)
		histogram.Observe(duration)

		if mapPublishRep.Disconnect != nil {
			return centrifuge.MapPublishReply{}, proxyproto.DisconnectFromProto(mapPublishRep.Disconnect)
		}
		if mapPublishRep.Error != nil {
			return centrifuge.MapPublishReply{}, proxyproto.ErrorFromProto(mapPublishRep.Error)
		}

		key := e.Key
		data := e.Data
		if mapPublishRep.Result != nil {
			if mapPublishRep.Result.Key != "" {
				key = mapPublishRep.Result.Key
			}
			if mapPublishRep.Result.Data != nil {
				data = mapPublishRep.Result.Data
			} else if mapPublishRep.Result.B64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(mapPublishRep.Result.B64Data)
				if err != nil {
					log.Error().Err(err).Str("client", client.ID()).Msg("error decoding base64 data")
					return centrifuge.MapPublishReply{}, centrifuge.ErrorInternal
				}
				data = decodedData
			}
		}

		result, err := node.MapPublish(
			client.Context(), e.Channel, key,
			centrifuge.MapPublishOptions{
				Data: data,
			},
		)
		return centrifuge.MapPublishReply{Result: &result}, err
	}
}
