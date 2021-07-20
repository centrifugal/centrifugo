package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"

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

// PublishHandlerFunc ...
type PublishHandlerFunc func(*centrifuge.Client, centrifuge.PublishEvent, rule.ChannelOptions) (centrifuge.PublishReply, error)

// Handle Publish.
func (h *PublishHandler) Handle(node *centrifuge.Node) PublishHandlerFunc {
	return func(client *centrifuge.Client, e centrifuge.PublishEvent, chOpts rule.ChannelOptions) (centrifuge.PublishReply, error) {
		started := time.Now()
		req := &proxyproto.PublishRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(h.config.Proxy.UseBase64()),

			User:    client.UserID(),
			Channel: e.Channel,
		}
		if !h.config.Proxy.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}

		publishRep, err := h.config.Proxy.ProxyPublish(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.PublishReply{}, nil
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying publish", map[string]interface{}{"error": err.Error()}))
			return centrifuge.PublishReply{}, centrifuge.ErrorInternal
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)

		if publishRep.Disconnect != nil {
			return centrifuge.PublishReply{}, proxyproto.DisconnectFromProto(publishRep.Disconnect)
		}
		if publishRep.Error != nil {
			return centrifuge.PublishReply{}, proxyproto.ErrorFromProto(publishRep.Error)
		}

		data := e.Data
		if publishRep.Result != nil {
			if publishRep.Result.Data != nil {
				data = publishRep.Result.Data
			} else if publishRep.Result.B64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(publishRep.Result.B64Data)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.PublishReply{}, centrifuge.ErrorInternal
				}
				data = decodedData
			}
		}

		result, err := node.Publish(
			e.Channel, data,
			centrifuge.WithClientInfo(e.ClientInfo),
			centrifuge.WithHistory(chOpts.HistorySize, time.Duration(chOpts.HistoryTTL)),
		)
		return centrifuge.PublishReply{Result: &result}, err
	}
}
