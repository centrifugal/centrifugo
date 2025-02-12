package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/clientstorage"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// ConnectHandlerConfig ...
type ConnectHandlerConfig struct {
	Proxy ConnectProxy
}

// ConnectHandler ...
type ConnectHandler struct {
	config       ConnectHandlerConfig
	cfgContainer *config.Container
	summary      prometheus.Observer
	histogram    prometheus.Observer
	errors       prometheus.Counter
	inflight     prometheus.Gauge
}

// NewConnectHandler ...
func NewConnectHandler(c ConnectHandlerConfig, cfgContainer *config.Container) *ConnectHandler {
	return &ConnectHandler{
		config:       c,
		cfgContainer: cfgContainer,
		summary:      proxyCallDurationSummary.WithLabelValues(c.Proxy.Protocol(), "connect", "default"),
		histogram:    proxyCallDurationHistogram.WithLabelValues(c.Proxy.Protocol(), "connect", "default"),
		errors:       proxyCallErrorCount.WithLabelValues(c.Proxy.Protocol(), "connect", "default"),
		inflight:     proxyCallInflightRequests.WithLabelValues(c.Proxy.Protocol(), "connect", "default"),
	}
}

type ConnectExtra struct {
}

type ConnectingHandlerFunc func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectReply, ConnectExtra, error)

// Handle returns connecting handler func.
func (h *ConnectHandler) Handle() ConnectingHandlerFunc {
	return func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, ConnectExtra, error) {
		started := time.Now()
		h.inflight.Inc()
		defer h.inflight.Dec()
		req := &proxyproto.ConnectRequest{
			Client:    e.ClientID,
			Protocol:  string(e.Transport.Protocol()),
			Transport: e.Transport.Name(),
			Encoding:  getEncoding(h.config.Proxy.UseBase64()),
			Name:      e.Name,
			Version:   e.Version,
			Channels:  e.Channels,
		}
		if !h.config.Proxy.UseBase64() {
			req.Data = e.Data
		} else {
			req.B64Data = base64.StdEncoding.EncodeToString(e.Data)
		}

		connectRep, err := h.config.Proxy.ProxyConnect(ctx, req)
		duration := time.Since(started).Seconds()
		if err != nil {
			select {
			case <-ctx.Done():
				// Client connection already closed.
				return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.DisconnectConnectionClosed
			default:
			}
			h.summary.Observe(duration)
			h.histogram.Observe(duration)
			h.errors.Inc()
			log.Error().Err(err).Str("client", e.ClientID).Msg("error proxying connect")
			return centrifuge.ConnectReply{}, ConnectExtra{}, err
		}
		h.summary.Observe(duration)
		h.histogram.Observe(duration)
		if connectRep.Disconnect != nil {
			return centrifuge.ConnectReply{}, ConnectExtra{}, proxyproto.DisconnectFromProto(connectRep.Disconnect)
		}
		if connectRep.Error != nil {
			return centrifuge.ConnectReply{}, ConnectExtra{}, proxyproto.ErrorFromProto(connectRep.Error)
		}

		result := connectRep.Result
		if result == nil {
			return centrifuge.ConnectReply{Credentials: nil}, ConnectExtra{}, nil
		}

		var info []byte
		if result.B64Info != "" {
			decodedInfo, err := base64.StdEncoding.DecodeString(result.B64Info)
			if err != nil {
				log.Error().Err(err).Str("client", e.ClientID).Msg("error decoding base64 info")
				return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorInternal
			}
			info = decodedInfo
		} else {
			info = result.Info
		}

		var data []byte
		if result.B64Data != "" {
			decodedData, err := base64.StdEncoding.DecodeString(result.B64Data)
			if err != nil {
				log.Error().Err(err).Str("client", e.ClientID).Msg("error decoding base64 data")
				return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorInternal
			}
			data = decodedData
		} else {
			data = result.Data
		}

		reply := centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   result.User,
				ExpireAt: result.ExpireAt,
				Info:     info,
			},
		}
		if len(data) > 0 {
			reply.Data = data
		}
		if len(result.Channels) > 0 {
			if reply.Subscriptions == nil {
				reply.Subscriptions = make(map[string]centrifuge.SubscribeOptions, len(result.Channels))
			}
			for _, ch := range result.Channels {
				_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
				if err != nil {
					return centrifuge.ConnectReply{}, ConnectExtra{}, err
				}
				if !found {
					log.Warn().Str("client", e.ClientID).Str("channel", ch).Msg("unknown channel in connect result channels")
					return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorUnknownChannel
				}
				reply.Subscriptions[ch] = centrifuge.SubscribeOptions{
					EmitPresence:      chOpts.Presence,
					EmitJoinLeave:     chOpts.JoinLeave,
					PushJoinLeave:     chOpts.ForcePushJoinLeave,
					EnableRecovery:    chOpts.ForceRecovery,
					EnablePositioning: chOpts.ForcePositioning,
					RecoveryMode:      chOpts.GetRecoveryMode(),
					Source:            subsource.ConnectProxy,
					HistoryMetaTTL:    chOpts.HistoryMetaTTL.ToDuration(),
					AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
				}
			}
		}
		if len(result.Subs) > 0 {
			if reply.Subscriptions == nil {
				reply.Subscriptions = make(map[string]centrifuge.SubscribeOptions, len(result.Subs))
			}
			for ch, options := range result.Subs {
				_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
				if err != nil {
					return centrifuge.ConnectReply{}, ConnectExtra{}, err
				}
				if !found {
					log.Warn().Str("client", e.ClientID).Str("channel", ch).Msg("unknown channel in connect result subs")
					return centrifuge.ConnectReply{}, ConnectExtra{}, centrifuge.ErrorUnknownChannel
				}
				var chInfo []byte
				if options.B64Info != "" {
					byteInfo, err := base64.StdEncoding.DecodeString(options.B64Info)
					if err != nil {
						return centrifuge.ConnectReply{}, ConnectExtra{}, err
					}
					chInfo = byteInfo
				} else {
					chInfo = options.Info
				}
				var chData []byte
				if options.B64Data != "" {
					byteInfo, err := base64.StdEncoding.DecodeString(options.B64Data)
					if err != nil {
						return centrifuge.ConnectReply{}, ConnectExtra{}, err
					}
					chData = byteInfo
				} else {
					chData = options.Data
				}
				presence := chOpts.Presence
				if options.Override != nil && options.Override.Presence != nil {
					presence = options.Override.Presence.Value
				}
				joinLeave := chOpts.JoinLeave
				if options.Override != nil && options.Override.JoinLeave != nil {
					joinLeave = options.Override.JoinLeave.Value
				}
				pushJoinLeave := chOpts.ForcePushJoinLeave
				if options.Override != nil && options.Override.ForcePushJoinLeave != nil {
					pushJoinLeave = options.Override.ForcePushJoinLeave.Value
				}
				recovery := chOpts.ForceRecovery
				if options.Override != nil && options.Override.ForceRecovery != nil {
					recovery = options.Override.ForceRecovery.Value
				}
				positioning := chOpts.ForcePositioning
				if options.Override != nil && options.Override.ForcePositioning != nil {
					positioning = options.Override.ForcePositioning.Value
				}
				recoveryMode := chOpts.GetRecoveryMode()
				reply.Subscriptions[ch] = centrifuge.SubscribeOptions{
					ChannelInfo:       chInfo,
					EmitPresence:      presence,
					EmitJoinLeave:     joinLeave,
					PushJoinLeave:     pushJoinLeave,
					EnableRecovery:    recovery,
					EnablePositioning: positioning,
					RecoveryMode:      recoveryMode,
					Data:              chData,
					Source:            subsource.ConnectProxy,
					HistoryMetaTTL:    chOpts.HistoryMetaTTL.ToDuration(),
					AllowedDeltaTypes: chOpts.AllowedDeltaTypes,
				}
			}
		}

		if result.Meta != nil {
			reply.Storage = map[string]any{
				clientstorage.KeyMeta: json.RawMessage(result.Meta),
			}
		}

		return reply, ConnectExtra{}, nil
	}
}
