package proxy

import (
	"encoding/base64"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// SubscribeHandlerConfig ...
type SubscribeHandlerConfig struct {
	Proxies map[string]SubscribeProxy
}

// SubscribeHandler ...
type SubscribeHandler struct {
	config    SubscribeHandlerConfig
	summary   map[string]prometheus.Observer
	histogram map[string]prometheus.Observer
	errors    map[string]prometheus.Counter
	inflight  map[string]prometheus.Gauge
}

// NewSubscribeHandler ...
func NewSubscribeHandler(c SubscribeHandlerConfig) *SubscribeHandler {
	h := &SubscribeHandler{
		config: c,
	}
	summary := map[string]prometheus.Observer{}
	histogram := map[string]prometheus.Observer{}
	errors := map[string]prometheus.Counter{}
	inflight := map[string]prometheus.Gauge{}
	for name, p := range c.Proxies {
		summary[name] = proxyCallDurationSummary.WithLabelValues(p.Protocol(), "subscribe", name)
		histogram[name] = proxyCallDurationHistogram.WithLabelValues(p.Protocol(), "subscribe", name)
		errors[name] = proxyCallErrorCount.WithLabelValues(p.Protocol(), "subscribe", name)
		inflight[name] = proxyCallInflightRequests.WithLabelValues(p.Protocol(), "subscribe", name)
	}
	h.summary = summary
	h.histogram = histogram
	h.errors = errors
	h.inflight = inflight
	return h
}

type SubscribeExtra struct {
}

// SubscribeHandlerFunc ...
type SubscribeHandlerFunc func(Client, centrifuge.SubscribeEvent, configtypes.ChannelOptions, PerCallData) (centrifuge.SubscribeReply, SubscribeExtra, error)

// Handle Subscribe.
func (h *SubscribeHandler) Handle() SubscribeHandlerFunc {
	return func(client Client, e centrifuge.SubscribeEvent, chOpts configtypes.ChannelOptions, pcd PerCallData) (centrifuge.SubscribeReply, SubscribeExtra, error) {
		started := time.Now()

		var p SubscribeProxy
		var summary prometheus.Observer
		var histogram prometheus.Observer
		var errors prometheus.Counter

		proxyEnabled := chOpts.SubscribeProxyEnabled
		proxyName := chOpts.SubscribeProxyName
		if !proxyEnabled {
			log.Info().Str("channel", e.Channel).Msg("subscribe proxy not enabled for a channel")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorNotAvailable
		}
		p = h.config.Proxies[proxyName]
		summary = h.summary[proxyName]
		histogram = h.histogram[proxyName]
		errors = h.errors[proxyName]
		inflight := h.inflight[proxyName]
		inflight.Inc()
		defer inflight.Dec()

		req := &proxyproto.SubscribeRequest{
			Client:    client.ID(),
			Protocol:  string(client.Transport().Protocol()),
			Transport: client.Transport().Name(),
			Encoding:  getEncoding(p.UseBase64()),

			User:    client.UserID(),
			Channel: e.Channel,
			Token:   e.Token,
		}
		if !p.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}
		if p.IncludeMeta() && pcd.Meta != nil {
			req.Meta = proxyproto.Raw(pcd.Meta)
		}
		subscribeRep, err := p.ProxySubscribe(client.Context(), req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-client.Context().Done():
				// Client connection already closed.
				return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			summary.Observe(duration)
			histogram.Observe(duration)
			errors.Inc()
			log.Error().Err(err).Str("client", client.ID()).Str("channel", e.Channel).Msg("error proxying subscribe")
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, err
		}
		summary.Observe(duration)
		histogram.Observe(duration)

		if subscribeRep.Disconnect != nil {
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, proxyproto.DisconnectFromProto(subscribeRep.Disconnect)
		}
		if subscribeRep.Error != nil {
			return centrifuge.SubscribeReply{}, SubscribeExtra{}, proxyproto.ErrorFromProto(subscribeRep.Error)
		}

		presence := chOpts.Presence
		joinLeave := chOpts.JoinLeave
		pushJoinLeave := chOpts.ForcePushJoinLeave
		recovery := chOpts.ForceRecovery
		positioning := chOpts.ForcePositioning
		recoveryMode := chOpts.GetRecoveryMode()

		var info []byte
		var data []byte
		var expireAt int64
		var extra SubscribeExtra
		if subscribeRep.Result != nil {
			if subscribeRep.Result.B64Info != "" {
				decodedInfo, err := base64.StdEncoding.DecodeString(subscribeRep.Result.B64Info)
				if err != nil {
					log.Error().Err(err).Str("client", client.ID()).Msg("error decoding base64 info")
					return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorInternal
				}
				info = decodedInfo
			} else {
				info = subscribeRep.Result.Info
			}
			if subscribeRep.Result.B64Data != "" {
				decodedData, err := base64.StdEncoding.DecodeString(subscribeRep.Result.B64Data)
				if err != nil {
					log.Error().Err(err).Str("client", client.ID()).Msg("error decoding base64 data")
					return centrifuge.SubscribeReply{}, SubscribeExtra{}, centrifuge.ErrorInternal
				}
				data = decodedData
			} else {
				data = subscribeRep.Result.Data
			}

			result := subscribeRep.Result

			if result.Override != nil && result.Override.Presence != nil {
				presence = result.Override.Presence.Value
			}
			if result.Override != nil && result.Override.JoinLeave != nil {
				joinLeave = result.Override.JoinLeave.Value
			}
			if result.Override != nil && result.Override.ForcePushJoinLeave != nil {
				pushJoinLeave = result.Override.ForcePushJoinLeave.Value
			}
			if result.Override != nil && result.Override.ForceRecovery != nil {
				recovery = result.Override.ForceRecovery.Value
			}
			if result.Override != nil && result.Override.ForcePositioning != nil {
				positioning = result.Override.ForcePositioning.Value
			}

			expireAt = result.ExpireAt
		}

		return centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ExpireAt:          expireAt,
				ChannelInfo:       info,
				EmitPresence:      presence,
				EmitJoinLeave:     joinLeave,
				PushJoinLeave:     pushJoinLeave,
				EnableRecovery:    recovery,
				EnablePositioning: positioning,
				RecoveryMode:      recoveryMode,
				AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
				Data:              data,
				Source:            subsource.SubscribeProxy,
				HistoryMetaTTL:    time.Duration(chOpts.HistoryMetaTTL),
			},
			ClientSideRefresh: true,
		}, extra, nil
	}
}
