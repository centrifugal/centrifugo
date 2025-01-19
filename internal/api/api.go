package api

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RPCHandler allows to handle custom RPC.
type RPCHandler func(ctx context.Context, params Raw) (Raw, error)

// Executor can run API methods.
type Executor struct {
	node         *centrifuge.Node
	cfgContainer *config.Container
	config       ExecutorConfig
	rpcExtension map[string]RPCHandler
	surveyCaller SurveyCaller
}

// SurveyCaller can do surveys.
type SurveyCaller interface {
	Channels(ctx context.Context, cmd *ChannelsRequest) (map[string]*ChannelInfo, error)
}

type ExecutorConfig struct {
	Protocol         string
	UseOpenTelemetry bool
}

// NewExecutor ...
func NewExecutor(n *centrifuge.Node, cfgContainer *config.Container, surveyCaller SurveyCaller, config ExecutorConfig) *Executor {
	e := &Executor{
		node:         n,
		cfgContainer: cfgContainer,
		config:       config,
		surveyCaller: surveyCaller,
		rpcExtension: make(map[string]RPCHandler),
	}
	return e
}

// SetRPCExtension ...
func (h *Executor) SetRPCExtension(method string, handler RPCHandler) {
	h.rpcExtension[method] = handler
}

func (h *Executor) processCmd(ctx context.Context, cmd *Command, i int, replies []*Reply) {
	var method string
	if cmd.Publish != nil {
		method = "publish"
		res := h.Publish(ctx, cmd.Publish)
		replies[i].Publish, replies[i].Error = res.Result, res.Error
	} else if cmd.Broadcast != nil {
		method = "broadcast"
		res := h.Broadcast(ctx, cmd.Broadcast)
		replies[i].Broadcast, replies[i].Error = res.Result, res.Error
	} else if cmd.Subscribe != nil {
		method = "subscribe"
		res := h.Subscribe(ctx, cmd.Subscribe)
		replies[i].Subscribe, replies[i].Error = res.Result, res.Error
	} else if cmd.Unsubscribe != nil {
		method = "unsubscribe"
		res := h.Unsubscribe(ctx, cmd.Unsubscribe)
		replies[i].Unsubscribe, replies[i].Error = res.Result, res.Error
	} else if cmd.Disconnect != nil {
		method = "disconnect"
		res := h.Disconnect(ctx, cmd.Disconnect)
		replies[i].Disconnect, replies[i].Error = res.Result, res.Error
	} else if cmd.History != nil {
		method = "history"
		res := h.History(ctx, cmd.History)
		replies[i].History, replies[i].Error = res.Result, res.Error
	} else if cmd.HistoryRemove != nil {
		method = "history_remove"
		res := h.HistoryRemove(ctx, cmd.HistoryRemove)
		replies[i].HistoryRemove, replies[i].Error = res.Result, res.Error
	} else if cmd.Presence != nil {
		method = "presence"
		res := h.Presence(ctx, cmd.Presence)
		replies[i].Presence, replies[i].Error = res.Result, res.Error
	} else if cmd.PresenceStats != nil {
		method = "presence_stats"
		res := h.PresenceStats(ctx, cmd.PresenceStats)
		replies[i].PresenceStats, replies[i].Error = res.Result, res.Error
	} else if cmd.Info != nil {
		method = "info"
		res := h.Info(ctx, cmd.Info)
		replies[i].Info, replies[i].Error = res.Result, res.Error
	} else if cmd.Rpc != nil {
		method = "rpc"
		res := h.RPC(ctx, cmd.Rpc)
		replies[i].Rpc, replies[i].Error = res.Result, res.Error
	} else if cmd.Refresh != nil {
		method = "refresh"
		res := h.Refresh(ctx, cmd.Refresh)
		replies[i].Refresh, replies[i].Error = res.Result, res.Error
	} else if cmd.Channels != nil {
		method = "channels"
		res := h.Channels(ctx, cmd.Channels)
		replies[i].Channels, replies[i].Error = res.Result, res.Error
	} else {
		method = "unknown"
		replies[i].Error = ErrorNotFound
	}
	if replies[i].Error != nil {
		incError(h.config.Protocol, method, replies[i].Error.Code)
	}
}

// batchRequestMaxConcurrency is applied for the parallel batch request.
const batchRequestMaxConcurrency = 1024

func (h *Executor) Batch(ctx context.Context, req *BatchRequest) *BatchResponse {
	replies := make([]*Reply, len(req.Commands))

	var sem chan struct{}
	if req.Parallel {
		sem = make(chan struct{}, batchRequestMaxConcurrency)
	}

	var wg sync.WaitGroup
	for i, cmd := range req.Commands {
		replies[i] = new(Reply)
		if !req.Parallel {
			h.processCmd(ctx, cmd, i, replies)
		} else {
			sem <- struct{}{}
			wg.Add(1)
			go func(i int, cmd *Command) {
				defer func() { <-sem }()
				defer wg.Done()
				h.processCmd(ctx, cmd, i, replies)
			}(i, cmd)
		}
	}
	wg.Wait()

	return &BatchResponse{Replies: replies}
}

// Publish publishes data into channel.
func (h *Executor) Publish(ctx context.Context, cmd *PublishRequest) *PublishResponse {
	defer observe(time.Now(), h.config.Protocol, "publish")

	ch := cmd.Channel

	if h.config.UseOpenTelemetry {
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(attribute.String("centrifugo.channel", ch))
	}

	resp := &PublishResponse{}

	if ch == "" {
		log.Error().Err(errors.New("channel required for publish")).Msg("bad publish request")
		resp.Error = ErrorBadRequest
		return resp
	}

	var data []byte
	if cmd.B64Data != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(cmd.B64Data)
		if err != nil {
			resp.Error = ErrorBadRequest
			return resp
		}
		data = byteInfo
	} else {
		data = cmd.Data
	}

	if len(data) == 0 {
		log.Error().Err(errors.New("data required for publish")).Msg("bad publish request")
		resp.Error = ErrorBadRequest
		return resp
	}

	_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorUnknownChannel
		return resp
	}

	historySize := chOpts.HistorySize
	historyTTL := chOpts.HistoryTTL
	historyMetaTTL := chOpts.HistoryMetaTTL
	if cmd.SkipHistory {
		historySize = 0
		historyTTL = configtypes.Duration(0)
	}

	delta := cmd.Delta
	if chOpts.DeltaPublish {
		delta = true
	}

	result, err := h.node.Publish(
		cmd.Channel, data,
		centrifuge.WithHistory(historySize, historyTTL.ToDuration(), historyMetaTTL.ToDuration()),
		centrifuge.WithTags(cmd.GetTags()),
		centrifuge.WithIdempotencyKey(cmd.GetIdempotencyKey()),
		centrifuge.WithDelta(delta),
	)
	if err != nil {
		log.Error().Err(err).Str("channel", cmd.Channel).Msg("error publishing data to channel")
		resp.Error = ErrorInternal
		return resp
	}
	resp.Result = &PublishResult{
		Offset: result.StreamPosition.Offset,
		Epoch:  result.StreamPosition.Epoch,
	}
	return resp
}

const broadcastRequestMaxConcurrency = 1024

// Broadcast publishes the same data into many channels.
func (h *Executor) Broadcast(ctx context.Context, cmd *BroadcastRequest) *BroadcastResponse {
	defer observe(time.Now(), h.config.Protocol, "broadcast")

	resp := &BroadcastResponse{}

	channels := cmd.Channels

	if h.config.UseOpenTelemetry {
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(attribute.Int("centrifugo.num_channels", len(channels)))
	}

	if len(channels) == 0 {
		log.Error().Err(errors.New("channels required for broadcast")).Msg("bad broadcast request")
		resp.Error = ErrorBadRequest
		return resp
	}

	var data []byte
	if cmd.B64Data != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(cmd.B64Data)
		if err != nil {
			resp.Error = ErrorBadRequest
			return resp
		}
		data = byteInfo
	} else {
		data = cmd.Data
	}

	if len(data) == 0 {
		log.Error().Err(errors.New("data required for broadcast")).Msg("bad broadcast request")
		resp.Error = ErrorBadRequest
		return resp
	}

	sem := make(chan struct{}, broadcastRequestMaxConcurrency)

	responses := make([]*PublishResponse, len(channels))
	var wg sync.WaitGroup
	wg.Add(len(channels))
	for i, ch := range channels {
		sem <- struct{}{}
		go func(i int, ch string) {
			defer func() { <-sem }()
			defer wg.Done()
			if ch == "" {
				respError := ErrorBadRequest
				incError(h.config.Protocol, "broadcast_publish", respError.Code)
				log.Error().Err(errors.New("channel can not be blank in broadcast")).Msg("bad broadcast request")
				responses[i] = &PublishResponse{Error: respError}
				return
			}

			_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
			if err != nil {
				respError := ErrorInternal
				incError(h.config.Protocol, "broadcast_publish", respError.Code)
				log.Error().Err(err).Str("channel", ch).Msg("error getting options for channel")
				responses[i] = &PublishResponse{Error: respError}
				return
			}
			if !found {
				respError := ErrorUnknownChannel
				incError(h.config.Protocol, "broadcast_publish", respError.Code)
				log.Error().Err(errors.New("channel not found")).Str("channel", ch).Msg("error getting options for channel")
				responses[i] = &PublishResponse{Error: respError}
				return
			}

			historySize := chOpts.HistorySize
			historyTTL := chOpts.HistoryTTL
			historyMetaTTL := chOpts.HistoryMetaTTL
			if cmd.SkipHistory {
				historySize = 0
				historyTTL = configtypes.Duration(0)
			}

			delta := cmd.Delta
			if chOpts.DeltaPublish {
				delta = true
			}

			result, err := h.node.Publish(
				ch, data,
				centrifuge.WithHistory(historySize, historyTTL.ToDuration(), historyMetaTTL.ToDuration()),
				centrifuge.WithTags(cmd.GetTags()),
				centrifuge.WithIdempotencyKey(cmd.GetIdempotencyKey()),
				centrifuge.WithDelta(delta),
			)
			resp := &PublishResponse{}
			if err == nil {
				resp.Result = &PublishResult{
					Offset: result.StreamPosition.Offset,
					Epoch:  result.StreamPosition.Epoch,
				}
			} else {
				respError := ErrorInternal
				incError(h.config.Protocol, "publish", respError.Code)
				log.Error().Err(err).Str("channel", ch).Msg("error publishing data to channel during broadcast")
				resp.Error = respError
			}
			responses[i] = resp
		}(i, ch)
	}
	wg.Wait()
	resp.Result = &BroadcastResult{Responses: responses}
	return resp
}

// Subscribe subscribes user to a channel and sends subscribe
// control message to other nodes, so they could also subscribe user.
func (h *Executor) Subscribe(_ context.Context, cmd *SubscribeRequest) *SubscribeResponse {
	defer observe(time.Now(), h.config.Protocol, "subscribe")

	resp := &SubscribeResponse{}

	user := cmd.User
	channel := cmd.Channel

	if channel == "" {
		log.Error().Err(errors.New("channel required for subscribe")).Msg("bad subscribe request")
		resp.Error = ErrorBadRequest
		return resp
	}

	_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(channel)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorUnknownChannel
		return resp
	}

	presence := chOpts.Presence
	if cmd.Override != nil && cmd.Override.Presence != nil {
		presence = cmd.Override.Presence.Value
	}
	joinLeave := chOpts.JoinLeave
	if cmd.Override != nil && cmd.Override.JoinLeave != nil {
		joinLeave = cmd.Override.JoinLeave.Value
	}
	pushJoinLeave := chOpts.ForcePushJoinLeave
	if cmd.Override != nil && cmd.Override.ForcePushJoinLeave != nil {
		pushJoinLeave = cmd.Override.ForcePushJoinLeave.Value
	}
	useRecover := chOpts.ForceRecovery
	if cmd.Override != nil && cmd.Override.ForceRecovery != nil {
		useRecover = cmd.Override.ForceRecovery.Value
	}
	position := chOpts.ForcePositioning
	if cmd.Override != nil && cmd.Override.ForcePositioning != nil {
		position = cmd.Override.ForcePositioning.Value
	}

	var recoverSince *centrifuge.StreamPosition
	if cmd.RecoverSince != nil {
		recoverSince = &centrifuge.StreamPosition{
			Offset: cmd.RecoverSince.Offset,
			Epoch:  cmd.RecoverSince.Epoch,
		}
	}

	err = h.node.Subscribe(user, channel,
		centrifuge.WithSubscribeData(cmd.Data),
		centrifuge.WithSubscribeClient(cmd.Client),
		centrifuge.WithSubscribeSession(cmd.Session),
		centrifuge.WithChannelInfo(cmd.Info),
		centrifuge.WithExpireAt(cmd.ExpireAt),
		centrifuge.WithEmitJoinLeave(joinLeave),
		centrifuge.WithPushJoinLeave(pushJoinLeave),
		centrifuge.WithRecovery(useRecover),
		centrifuge.WithPositioning(position),
		centrifuge.WithEmitPresence(presence),
		centrifuge.WithRecoverSince(recoverSince),
		centrifuge.WithSubscribeSource(subsource.ServerAPI),
		centrifuge.WithSubscribeHistoryMetaTTL(chOpts.HistoryMetaTTL.ToDuration()),
	)
	if err != nil {
		log.Error().Err(err).Str("channel", channel).Str("user", user).Msg("error subscribing user to channel")
		resp.Error = ErrorInternal
		return resp
	}
	resp.Result = &SubscribeResult{}
	return resp
}

// Unsubscribe unsubscribes user from channel and sends unsubscribe
// control message to other nodes, so they could also unsubscribe user.
func (h *Executor) Unsubscribe(_ context.Context, cmd *UnsubscribeRequest) *UnsubscribeResponse {
	defer observe(time.Now(), h.config.Protocol, "unsubscribe")

	resp := &UnsubscribeResponse{}

	user := cmd.User
	channel := cmd.Channel

	if channel != "" {
		_, _, _, found, err := h.cfgContainer.ChannelOptions(channel)
		if err != nil {
			resp.Error = ErrorInternal
			return resp
		}
		if !found {
			resp.Error = ErrorUnknownChannel
			return resp
		}
	}

	err := h.node.Unsubscribe(user, channel, centrifuge.WithUnsubscribeClient(cmd.Client), centrifuge.WithUnsubscribeSession(cmd.Session))
	if err != nil {
		log.Error().Err(err).Str("channel", channel).Str("user", user).Msg("error unsubscribing user from channel")
		resp.Error = ErrorInternal
		return resp
	}
	resp.Result = &UnsubscribeResult{}
	return resp
}

// Disconnect disconnects user by its ID and sends disconnect
// control message to other nodes, so they could also disconnect user.
func (h *Executor) Disconnect(_ context.Context, cmd *DisconnectRequest) *DisconnectResponse {
	defer observe(time.Now(), h.config.Protocol, "disconnect")

	resp := &DisconnectResponse{}

	user := cmd.User

	disconnect := centrifuge.DisconnectForceNoReconnect
	if cmd.Disconnect != nil {
		disconnect = centrifuge.Disconnect{
			Code:   cmd.Disconnect.Code,
			Reason: cmd.Disconnect.Reason,
		}
	}

	err := h.node.Disconnect(
		user,
		centrifuge.WithCustomDisconnect(disconnect),
		centrifuge.WithDisconnectClient(cmd.Client),
		centrifuge.WithDisconnectSession(cmd.Session),
		centrifuge.WithDisconnectClientWhitelist(cmd.Whitelist))
	if err != nil {
		log.Error().Err(err).Str("user", user).Msg("error disconnecting user")
		resp.Error = ErrorInternal
		return resp
	}
	resp.Result = &DisconnectResult{}
	return resp
}

// Refresh user connection by its ID.
func (h *Executor) Refresh(_ context.Context, cmd *RefreshRequest) *RefreshResponse {
	defer observe(time.Now(), h.config.Protocol, "refresh")

	resp := &RefreshResponse{}
	user := cmd.User

	err := h.node.Refresh(
		user,
		centrifuge.WithRefreshClient(cmd.Client),
		centrifuge.WithRefreshSession(cmd.Session),
		centrifuge.WithRefreshExpired(cmd.Expired),
		centrifuge.WithRefreshExpireAt(cmd.ExpireAt),
		centrifuge.WithRefreshInfo(cmd.Info),
	)
	if err != nil {
		log.Error().Err(err).Str("user", user).Msg("error refreshing user")
		resp.Error = ErrorInternal
		return resp
	}
	resp.Result = &RefreshResult{}
	return resp
}

// Presence returns response with presence information for channel.
func (h *Executor) Presence(_ context.Context, cmd *PresenceRequest) *PresenceResponse {
	defer observe(time.Now(), h.config.Protocol, "presence")

	resp := &PresenceResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorUnknownChannel
		return resp
	}

	if !chOpts.Presence {
		resp.Error = ErrorNotAvailable
		return resp
	}

	presence, err := h.node.Presence(ch)
	if err != nil {
		log.Error().Err(err).Str("channel", ch).Msg("error getting presence for channel")
		resp.Error = ErrorInternal
		return resp
	}

	apiPresence := make(map[string]*ClientInfo, len(presence.Presence))
	for k, v := range presence.Presence {
		apiPresence[k] = &ClientInfo{
			User:     v.UserID,
			Client:   v.ClientID,
			ConnInfo: v.ConnInfo,
			ChanInfo: v.ChanInfo,
		}
	}

	resp.Result = &PresenceResult{
		Presence: apiPresence,
	}
	return resp
}

// PresenceStats returns response with presence stats information for channel.
func (h *Executor) PresenceStats(_ context.Context, cmd *PresenceStatsRequest) *PresenceStatsResponse {
	defer observe(time.Now(), h.config.Protocol, "presence_stats")

	resp := &PresenceStatsResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorUnknownChannel
		return resp
	}

	if !chOpts.Presence {
		resp.Error = ErrorNotAvailable
		return resp
	}

	stats, err := h.node.PresenceStats(cmd.Channel)
	if err != nil {
		log.Error().Err(err).Str("channel", ch).Msg("error getting presence stats for channel")
		resp.Error = ErrorInternal
		return resp
	}

	resp.Result = &PresenceStatsResult{
		NumClients: uint32(stats.NumClients),
		NumUsers:   uint32(stats.NumUsers),
	}

	return resp
}

// History returns response with history information for channel.
func (h *Executor) History(_ context.Context, cmd *HistoryRequest) *HistoryResponse {
	defer observe(time.Now(), h.config.Protocol, "history")

	resp := &HistoryResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorUnknownChannel
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryTTL <= 0 {
		resp.Error = ErrorNotAvailable
		return resp
	}

	var sp *centrifuge.StreamPosition
	if cmd.Since != nil {
		sp = &centrifuge.StreamPosition{
			Epoch:  cmd.Since.Epoch,
			Offset: cmd.Since.Offset,
		}
	}

	historyMetaTTL := chOpts.HistoryMetaTTL

	history, err := h.node.History(
		ch,
		centrifuge.WithHistoryMetaTTL(historyMetaTTL.ToDuration()),
		centrifuge.WithLimit(int(cmd.Limit)),
		centrifuge.WithSince(sp),
		centrifuge.WithReverse(cmd.Reverse),
	)
	if err != nil {
		log.Error().Err(err).Str("channel", ch).Msg("error getting history for channel")
		if errors.Is(err, centrifuge.ErrorUnrecoverablePosition) {
			resp.Error = ErrorUnrecoverablePosition
			return resp
		}
		resp.Error = ErrorInternal
		return resp
	}

	apiPubs := make([]*Publication, len(history.Publications))

	for i, pub := range history.Publications {
		apiPub := &Publication{
			Data:   Raw(pub.Data),
			Offset: pub.Offset,
			Tags:   pub.Tags,
		}
		if pub.Info != nil {
			apiPub.Info = &ClientInfo{
				User:     pub.Info.UserID,
				Client:   pub.Info.ClientID,
				ConnInfo: pub.Info.ConnInfo,
				ChanInfo: pub.Info.ChanInfo,
			}
		}
		apiPubs[i] = apiPub
	}

	resp.Result = &HistoryResult{
		Publications: apiPubs,
		Offset:       history.Offset,
		Epoch:        history.Epoch,
	}
	return resp
}

// HistoryRemove removes all history information for channel.
func (h *Executor) HistoryRemove(_ context.Context, cmd *HistoryRemoveRequest) *HistoryRemoveResponse {
	defer observe(time.Now(), h.config.Protocol, "history_remove")

	resp := &HistoryRemoveResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	_, _, chOpts, found, err := h.cfgContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorUnknownChannel
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryTTL <= 0 {
		resp.Error = ErrorNotAvailable
		return resp
	}

	err = h.node.RemoveHistory(ch)
	if err != nil {
		log.Error().Err(err).Str("channel", ch).Msg("error removing history for channel")
		resp.Error = ErrorInternal
		return resp
	}
	resp.Result = &HistoryRemoveResult{}
	return resp
}

// Info returns information about running nodes.
func (h *Executor) Info(_ context.Context, _ *InfoRequest) *InfoResponse {
	defer observe(time.Now(), h.config.Protocol, "info")

	resp := &InfoResponse{}

	info, err := h.node.Info()
	if err != nil {
		log.Error().Err(err).Msg("error calling info")
		resp.Error = ErrorInternal
		return resp
	}

	nodes := make([]*NodeResult, len(info.Nodes))
	for i, nd := range info.Nodes {
		res := &NodeResult{
			Uid:         nd.UID,
			Version:     nd.Version,
			Name:        nd.Name,
			NumClients:  nd.NumClients,
			NumUsers:    nd.NumUsers,
			NumSubs:     nd.NumSubs,
			NumChannels: nd.NumChannels,
			Uptime:      nd.Uptime,
			Process:     nil,
		}
		if nd.Metrics != nil {
			res.Metrics = &Metrics{
				Interval: nd.Metrics.Interval,
				Items:    nd.Metrics.Items,
			}
		}
		nodes[i] = res
	}
	resp.Result = &InfoResult{
		Nodes: nodes,
	}
	return resp
}

// RPC can call arbitrary methods.
func (h *Executor) RPC(ctx context.Context, cmd *RPCRequest) *RPCResponse {
	started := time.Now()
	defer observe(started, h.config.Protocol, "rpc")

	resp := &RPCResponse{}

	if cmd.Method == "" {
		log.Error().Err(errors.New("rpc method required")).Msg("bad rpc request")
		resp.Error = ErrorBadRequest
		return resp
	}

	handler, ok := h.rpcExtension[cmd.Method]
	if !ok {
		resp.Error = ErrorNotFound
		return resp
	}

	defer observeRPC(started, h.config.Protocol, cmd.Method)

	data, err := handler(ctx, cmd.Params)
	if err != nil {
		log.Error().Err(err).Str("method", cmd.Method).Msg("error calling rpc method")
		resp.Error = toAPIErr(err)
		return resp
	}

	resp.Result = &RPCResult{
		Data: data,
	}

	return resp
}

// Channels in the system.
func (h *Executor) Channels(ctx context.Context, cmd *ChannelsRequest) *ChannelsResponse {
	started := time.Now()
	defer observe(started, h.config.Protocol, "channels")

	resp := &ChannelsResponse{}

	channels, err := h.surveyCaller.Channels(ctx, cmd)
	if err != nil {
		log.Error().Err(err).Msg("error calling channels")
		resp.Error = toAPIErr(err)
		return resp
	}

	resp.Result = &ChannelsResult{
		Channels: channels,
	}

	return resp
}

func toAPIErr(err error) *Error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return ErrorInternal
}
