package proxy

import (
	"context"
	"errors"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// PublishHandlerConfig ...
type PublishHandlerConfig struct {
	Proxy PublishProxy
}

// PublishHandler ...
type PublishHandler struct {
	config    PublishHandlerConfig
	summary   prometheus.Observer
	histogram prometheus.Observer
	errors    prometheus.Counter
}

// NewPublishHandler ...
func NewPublishHandler(c PublishHandlerConfig) *PublishHandler {
	return &PublishHandler{
		config:    c,
		summary:   proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "publish"),
		histogram: proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "publish"),
		errors:    proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "publish"),
	}
}

// Handle Publish.
func (h *PublishHandler) Handle(ctx context.Context, node *centrifuge.Node, client *centrifuge.Client) func(e centrifuge.PublishEvent) centrifuge.PublishReply {
	return func(e centrifuge.PublishEvent) centrifuge.PublishReply {
		started := time.Now()
		publishRep, err := h.config.Proxy.ProxyPublish(ctx, PublishRequest{
			ClientID:  client.ID(),
			UserID:    client.UserID(),
			Channel:   e.Channel,
			Data:      e.Data,
			Transport: client.Transport(),
		})
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.PublishReply{}
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying publish", map[string]interface{}{"error": err.Error()}))
			return centrifuge.PublishReply{
				Error: centrifuge.ErrorInternal,
			}
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)

		if publishRep.Disconnect != nil {
			return centrifuge.PublishReply{
				Disconnect: publishRep.Disconnect,
			}
		}
		if publishRep.Error != nil {
			return centrifuge.PublishReply{
				Error: publishRep.Error,
			}
		}

		return centrifuge.PublishReply{}
	}
}
