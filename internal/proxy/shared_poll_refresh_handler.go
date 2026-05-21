package proxy

import (
	"context"
	"errors"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/metrics"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// SharedPollRefreshHandlerConfig ...
type SharedPollRefreshHandlerConfig struct {
	Proxy SharedPollRefreshProxy
	Name  string
}

// SharedPollRefreshHandler ...
type SharedPollRefreshHandler struct {
	proxy         SharedPollRefreshProxy
	summary       prometheus.Observer
	histogram     prometheus.Observer
	errors        prometheus.Counter
	requestItems  prometheus.Observer
	responseItems prometheus.Observer
}

// NewSharedPollRefreshHandler ...
func NewSharedPollRefreshHandler(c SharedPollRefreshHandlerConfig) *SharedPollRefreshHandler {
	return &SharedPollRefreshHandler{
		proxy:         c.Proxy,
		summary:       metrics.ProxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "shared_poll_refresh", c.Name),
		histogram:     metrics.ProxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "shared_poll_refresh", c.Name),
		errors:        metrics.ProxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "shared_poll_refresh", c.Name),
		requestItems:  metrics.SharedPollProxyRequestItems.WithLabelValues(c.Name),
		responseItems: metrics.SharedPollProxyResponseItems.WithLabelValues(c.Name),
	}
}

// Handle returns a centrifuge.SharedPollHandler.
func (h *SharedPollRefreshHandler) Handle(node *centrifuge.Node) centrifuge.SharedPollHandler {
	return func(ctx context.Context, event centrifuge.SharedPollEvent) (centrifuge.SharedPollResult, error) {
		started := time.Now()

		protoItems := make([]*proxyproto.SharedPollRefreshItem, len(event.Items))
		for i, item := range event.Items {
			protoItems[i] = &proxyproto.SharedPollRefreshItem{
				Key:     item.Key,
				Version: item.Version,
			}
		}
		req := &proxyproto.SharedPollRefreshRequest{
			Channel: event.Channel,
			Items:   protoItems,
		}
		h.requestItems.Observe(float64(len(event.Items)))

		resp, err := h.proxy.ProxySharedPollRefresh(ctx, req)
		duration := time.Since(started).Seconds()
		h.summary.Observe(duration)
		h.histogram.Observe(duration)
		if err != nil {
			h.errors.Inc()
			log.Error().Err(err).Str("channel", event.Channel).Msg("error proxying shared poll refresh")
			return centrifuge.SharedPollResult{}, err
		}
		if resp.Error != nil {
			h.errors.Inc()
			return centrifuge.SharedPollResult{}, errors.New(resp.Error.Message)
		}
		if resp.Result == nil {
			return centrifuge.SharedPollResult{}, nil
		}

		h.responseItems.Observe(float64(len(resp.Result.Items)))
		items := make([]centrifuge.SharedPollRefreshItem, len(resp.Result.Items))
		for i, item := range resp.Result.Items {
			items[i] = centrifuge.SharedPollRefreshItem{
				Key:      item.Key,
				Data:     item.Data,
				PrevData: item.PrevData,
				Version:  item.Version,
				Removed:  item.Removed,
			}
		}
		return centrifuge.SharedPollResult{Items: items, Epoch: resp.Result.Epoch}, nil
	}
}
