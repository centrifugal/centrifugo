package proxy

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// RefreshHandlerConfig ...
type RefreshHandlerConfig struct {
	Proxy RefreshProxy
}

// RefreshHandler ...
type RefreshHandler struct {
	config    RefreshHandlerConfig
	summary   prometheus.Observer
	histogram prometheus.Observer
	errors    prometheus.Counter
	inflight  prometheus.Gauge
}

// NewRefreshHandler ...
func NewRefreshHandler(c RefreshHandlerConfig) *RefreshHandler {
	return &RefreshHandler{
		config:    c,
		summary:   proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "refresh", "default"),
		histogram: proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "refresh", "default"),
		errors:    proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "refresh", "default"),
		inflight:  proxyCallInflightRequests.WithLabelValues(c.Proxy.Protocol(), "refresh", "default"),
	}
}

type RefreshExtra struct {
	Meta json.RawMessage
}

// RefreshHandlerFunc ...
type RefreshHandlerFunc func(Client, centrifuge.RefreshEvent, PerCallData) (centrifuge.RefreshReply, RefreshExtra, error)

// Handle refresh.
func (h *RefreshHandler) Handle() RefreshHandlerFunc {
	return func(client Client, e centrifuge.RefreshEvent, pcd PerCallData) (centrifuge.RefreshReply, RefreshExtra, error) {
		started := time.Now()
		h.inflight.Inc()
		defer h.inflight.Dec()
		req := &proxyproto.RefreshRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(h.config.Proxy.UseBase64()),

			User: client.UserID(),
		}
		if h.config.Proxy.IncludeMeta() && pcd.Meta != nil {
			req.Meta = proxyproto.Raw(pcd.Meta)
		}
		refreshRep, err := h.config.Proxy.ProxyRefresh(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.RefreshReply{}, RefreshExtra{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			log.Error().Err(err).Msg("error proxying refresh")
			// In case of an error give connection one more minute to live and
			// then try to check again. This way we gracefully handle temporary
			// problems on application backend side.
			// NOTE: this interval must be configurable maybe, but for now looks
			// like a reasonable value.
			return centrifuge.RefreshReply{
				ExpireAt: time.Now().Unix() + 60,
			}, RefreshExtra{}, nil
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)

		result := refreshRep.Result
		if result == nil {
			// User will be disconnected.
			log.Error().Msg("no refresh result found")
			return centrifuge.RefreshReply{
				Expired: true,
			}, RefreshExtra{}, nil
		}

		if result.Expired {
			return centrifuge.RefreshReply{
				Expired: true,
			}, RefreshExtra{}, nil
		}

		var info []byte
		if result.B64Info != "" {
			decodedInfo, err := base64.StdEncoding.DecodeString(result.B64Info)
			if err != nil {
				log.Error().Err(err).Str("client", client.ID()).Msg("error decoding base64 info")
				return centrifuge.RefreshReply{}, RefreshExtra{}, centrifuge.ErrorInternal
			}
			info = decodedInfo
		} else {
			info = result.Info
		}

		extra := RefreshExtra{}
		if result.Meta != nil {
			extra.Meta = json.RawMessage(result.Meta)
		}

		return centrifuge.RefreshReply{
			ExpireAt: result.ExpireAt,
			Info:     info,
		}, extra, nil
	}
}
