package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/v5/internal/proxyproto"
	"github.com/prometheus/client_golang/prometheus"
)

// PublishHandlerConfig ...
type PublishHandlerConfig struct {
	Proxies           map[string]PublishProxy
	GranularProxyMode bool
}

// PublishHandler ...
type PublishHandler struct {
	config            PublishHandlerConfig
	summary           prometheus.Observer
	histogram         prometheus.Observer
	errors            prometheus.Counter
	granularSummary   map[string]prometheus.Observer
	granularHistogram map[string]prometheus.Observer
	granularErrors    map[string]prometheus.Counter
}

// NewPublishHandler ...
func NewPublishHandler(c PublishHandlerConfig) *PublishHandler {
	h := &PublishHandler{
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
			summary[name] = granularProxyCallDurationSummary.WithLabelValues("publish", name)
			histogram[name] = granularProxyCallDurationHistogram.WithLabelValues("publish", name)
			errors[name] = granularProxyCallErrorCount.WithLabelValues("publish", name)
		}
		h.granularSummary = summary
		h.granularHistogram = histogram
		h.granularErrors = errors
	} else {
		h.summary = proxyCallDurationSummary.WithLabelValues(h.config.Proxies[""].Protocol(), "publish")
		h.histogram = proxyCallDurationHistogram.WithLabelValues(h.config.Proxies[""].Protocol(), "publish")
		h.errors = proxyCallErrorCount.WithLabelValues(h.config.Proxies[""].Protocol(), "publish")
	}
	return h
}

// PublishHandlerFunc ...
type PublishHandlerFunc func(Client, centrifuge.PublishEvent, configtypes.ChannelOptions, PerCallData) (centrifuge.PublishReply, error)

// Handle Publish.
func (h *PublishHandler) Handle(node *centrifuge.Node) PublishHandlerFunc {
	return func(client Client, e centrifuge.PublishEvent, chOpts configtypes.ChannelOptions, pcd PerCallData) (centrifuge.PublishReply, error) {
		started := time.Now()

		var p PublishProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		if h.config.GranularProxyMode {
			proxyName := chOpts.PublishProxyName
			if proxyName == "" {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "publish proxy not configured for a channel", map[string]any{"channel": e.Channel}))
				return centrifuge.PublishReply{}, centrifuge.ErrorNotAvailable
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

		req := &proxyproto.PublishRequest{
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
		if !p.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}

		publishRep, err := p.ProxyPublish(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.PublishReply{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying publish", map[string]any{"error": err.Error()}))
			return centrifuge.PublishReply{}, centrifuge.ErrorInternal
		}
		summary.Observe(duration)
		histogram.Observe(duration)

		if publishRep.Disconnect != nil {
			return centrifuge.PublishReply{}, proxyproto.DisconnectFromProto(publishRep.Disconnect)
		}
		if publishRep.Error != nil {
			return centrifuge.PublishReply{}, proxyproto.ErrorFromProto(publishRep.Error)
		}

		historySize := chOpts.HistorySize
		historyTTL := chOpts.HistoryTTL
		historyMetaTTL := chOpts.HistoryMetaTTL

		data := e.Data
		if publishRep.Result != nil {
			if publishRep.Result.Data != nil {
				data = publishRep.Result.Data
			} else if publishRep.Result.B64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(publishRep.Result.B64Data)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 data", map[string]any{"client": client.ID(), "error": err.Error()}))
					return centrifuge.PublishReply{}, centrifuge.ErrorInternal
				}
				data = decodedData
			}

			if publishRep.Result.SkipHistory {
				historySize = 0
				historyTTL = 0
			}
		}

		result, err := node.Publish(
			e.Channel, data,
			centrifuge.WithClientInfo(e.ClientInfo),
			centrifuge.WithHistory(historySize, time.Duration(historyTTL), time.Duration(historyMetaTTL)),
		)
		return centrifuge.PublishReply{Result: &result}, err
	}
}
