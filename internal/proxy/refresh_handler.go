package proxy

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
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
}

// NewRefreshHandler ...
func NewRefreshHandler(c RefreshHandlerConfig) *RefreshHandler {
	return &RefreshHandler{
		config:    c,
		summary:   proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "refresh"),
		histogram: proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "refresh"),
		errors:    proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "refresh"),
	}
}

// Handle refresh.
func (h *RefreshHandler) Handle(node *centrifuge.Node) func(context.Context, *centrifuge.Client, centrifuge.RefreshEvent) (centrifuge.RefreshReply, error) {
	return func(ctx context.Context, client *centrifuge.Client, e centrifuge.RefreshEvent) (centrifuge.RefreshReply, error) {
		started := time.Now()
		refreshRep, err := h.config.Proxy.ProxyRefresh(ctx, RefreshRequest{
			ClientID:  client.ID(),
			UserID:    client.UserID(),
			Transport: client.Transport(),
		})
		duration := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return centrifuge.RefreshReply{}, nil
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error proxying refresh", map[string]interface{}{"error": err.Error()}))
			// In case of an error give connection one more minute to live and
			// then try to check again. This way we gracefully handle temporary
			// problems on application backend side.
			// NOTE: this interval must be configurable maybe, but for now looks
			// like a reasonable value.
			return centrifuge.RefreshReply{
				ExpireAt: time.Now().Unix() + 60,
			}, nil
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)

		credentials := refreshRep.Result
		if credentials == nil {
			// User will be disconnected.
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "no refresh credentials found", map[string]interface{}{}))
			return centrifuge.RefreshReply{
				Expired: true,
			}, nil
		}

		if credentials.Expired {
			return centrifuge.RefreshReply{
				Expired: true,
			}, nil
		}

		var info []byte
		if client.Transport().Encoding() == "json" {
			info = credentials.Info
		} else {
			if credentials.Base64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(credentials.Base64Info)
				if err != nil {
					node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error decoding base64 info", map[string]interface{}{"client": client.ID(), "error": err.Error()}))
					return centrifuge.RefreshReply{}, centrifuge.ErrorInternal
				}
				info = decodedInfo
			}
		}

		return centrifuge.RefreshReply{
			ExpireAt: credentials.ExpireAt,
			Info:     info,
		}, nil
	}
}
