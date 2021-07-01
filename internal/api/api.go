package api

import (
	"context"
	"encoding/base64"
	"sync"
	"time"

	. "github.com/centrifugal/centrifugo/v3/internal/apiproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"

	"github.com/centrifugal/centrifuge"
)

// RPCHandler allows to handle custom RPC.
type RPCHandler func(ctx context.Context, params Raw) (Raw, error)

// Executor can run API methods.
type Executor struct {
	node          *centrifuge.Node
	ruleContainer *rule.Container
	protocol      string
	rpcExtension  map[string]RPCHandler
	surveyCaller  SurveyCaller
}

// SurveyCaller can do surveys.
type SurveyCaller interface {
	Channels(ctx context.Context, params Raw) (Raw, error)
}

// NewExecutor ...
func NewExecutor(n *centrifuge.Node, ruleContainer *rule.Container, surveyCaller SurveyCaller, protocol string) *Executor {
	e := &Executor{
		node:          n,
		ruleContainer: ruleContainer,
		protocol:      protocol,
		surveyCaller:  surveyCaller,
		rpcExtension:  make(map[string]RPCHandler),
	}
	e.SetRPCExtension("getChannels", func(ctx context.Context, params Raw) (Raw, error) {
		return surveyCaller.Channels(ctx, params)
	})
	return e
}

// SetRPCExtension ...
func (h Executor) SetRPCExtension(method string, handler RPCHandler) {
	h.rpcExtension[method] = handler
}

// Publish publishes data into channel.
func (h *Executor) Publish(_ context.Context, cmd *PublishRequest) *PublishResponse {
	defer observe(time.Now(), h.protocol, "publish")

	ch := cmd.Channel

	resp := &PublishResponse{}

	if ch == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel required for publish", nil))
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
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "data required for publish", nil))
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	historySize := chOpts.HistorySize
	historyTTL := chOpts.HistoryTTL
	if cmd.SkipHistory {
		historySize = 0
		historyTTL = 0
	}

	result, err := h.node.Publish(
		cmd.Channel, cmd.Data,
		centrifuge.WithHistory(historySize, time.Duration(historyTTL)*time.Second),
	)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error publishing message in engine", map[string]interface{}{"error": err.Error(), "channel": cmd.Channel}))
		resp.Error = ErrorInternal
		return resp
	}
	resp.Result = &PublishResult{
		Offset: result.StreamPosition.Offset,
		Epoch:  result.StreamPosition.Epoch,
	}
	return resp
}

// Broadcast publishes the same data into many channels.
func (h *Executor) Broadcast(_ context.Context, cmd *BroadcastRequest) *BroadcastResponse {
	defer observe(time.Now(), h.protocol, "broadcast")

	resp := &BroadcastResponse{}

	channels := cmd.Channels

	if len(channels) == 0 {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channels required for broadcast", nil))
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
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "data required for broadcast", nil))
		resp.Error = ErrorBadRequest
		return resp
	}

	responses := make([]*PublishResponse, len(channels))
	var wg sync.WaitGroup
	wg.Add(len(channels))
	for i, ch := range channels {
		go func(i int, ch string) {
			defer wg.Done()
			if ch == "" {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel can not be blank in broadcast", nil))
				responses[i] = &PublishResponse{Error: ErrorBadRequest}
				return
			}

			chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error getting options for channel", map[string]interface{}{"channel": ch, "error": err.Error()}))
				responses[i] = &PublishResponse{Error: ErrorInternal}
				return
			}
			if !found {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "can't find namespace for channel", map[string]interface{}{"channel": ch}))
				responses[i] = &PublishResponse{Error: ErrorNamespaceNotFound}
				return
			}

			historySize := chOpts.HistorySize
			historyTTL := chOpts.HistoryTTL
			if cmd.SkipHistory {
				historySize = 0
				historyTTL = 0
			}

			result, err := h.node.Publish(
				ch, data,
				centrifuge.WithHistory(historySize, time.Duration(historyTTL)*time.Second),
			)
			resp := &PublishResponse{}
			if err == nil {
				resp.Result = &PublishResult{
					Offset: result.StreamPosition.Offset,
					Epoch:  result.StreamPosition.Epoch,
				}
			} else {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error publishing data to channel", map[string]interface{}{"channel": ch, "error": err.Error()}))
				resp.Error = ErrorInternal
			}
			responses[i] = resp
		}(i, ch)
	}
	wg.Wait()
	resp.Result = &BroadcastResult{Responses: responses}
	return resp
}

// Subscribe subscribes user to a channel and sends subscribe
// control message to other nodes so they could also subscribe user.
func (h *Executor) Subscribe(_ context.Context, cmd *SubscribeRequest) *SubscribeResponse {
	defer observe(time.Now(), h.protocol, "subscribe")

	resp := &SubscribeResponse{}

	user := cmd.User
	channel := cmd.Channel

	if user == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "user required for subscribe", map[string]interface{}{"channel": channel, "user": user}))
		resp.Error = ErrorBadRequest
		return resp
	}

	if channel == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel required for subscribe", map[string]interface{}{"channel": channel, "user": user}))
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, found, err := h.ruleContainer.ChannelOptions(channel)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
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
	useRecover := chOpts.Recover
	if cmd.Override != nil && cmd.Override.Recover != nil {
		useRecover = cmd.Override.Recover.Value
	}
	position := chOpts.Position
	if cmd.Override != nil && cmd.Override.Position != nil {
		position = cmd.Override.Position.Value
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
		centrifuge.WithChannelInfo(cmd.Info),
		centrifuge.WithExpireAt(cmd.ExpireAt),
		centrifuge.WithJoinLeave(joinLeave),
		centrifuge.WithRecover(useRecover),
		centrifuge.WithPosition(position),
		centrifuge.WithPresence(presence),
		centrifuge.WithRecoverSince(recoverSince),
	)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error subscribing user to a channel", map[string]interface{}{"channel": channel, "user": user, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}
	return resp
}

// Unsubscribe unsubscribes user from channel and sends unsubscribe
// control message to other nodes so they could also unsubscribe user.
func (h *Executor) Unsubscribe(_ context.Context, cmd *UnsubscribeRequest) *UnsubscribeResponse {
	defer observe(time.Now(), h.protocol, "unsubscribe")

	resp := &UnsubscribeResponse{}

	user := cmd.User
	channel := cmd.Channel

	if user == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "user required for unsubscribe", map[string]interface{}{"channel": channel, "user": user}))
		resp.Error = ErrorBadRequest
		return resp
	}

	if channel != "" {
		_, found, err := h.ruleContainer.ChannelOptions(channel)
		if err != nil {
			resp.Error = ErrorInternal
			return resp
		}
		if !found {
			resp.Error = ErrorNamespaceNotFound
			return resp
		}
	}

	err := h.node.Unsubscribe(user, channel, centrifuge.WithUnsubscribeClient(cmd.Client))
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error unsubscribing user from a channel", map[string]interface{}{"channel": channel, "user": user, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}
	return resp
}

// Disconnect disconnects user by its ID and sends disconnect
// control message to other nodes so they could also disconnect user.
func (h *Executor) Disconnect(_ context.Context, cmd *DisconnectRequest) *DisconnectResponse {
	defer observe(time.Now(), h.protocol, "disconnect")

	resp := &DisconnectResponse{}

	user := cmd.User
	if user == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "user required for disconnect"))
		resp.Error = ErrorBadRequest
		return resp
	}

	var disconnect *centrifuge.Disconnect
	if cmd.Disconnect == nil {
		disconnect = centrifuge.DisconnectForceNoReconnect
	} else {
		disconnect = &centrifuge.Disconnect{
			Code:      cmd.Disconnect.Code,
			Reason:    cmd.Disconnect.Reason,
			Reconnect: cmd.Disconnect.Reconnect,
		}
	}

	err := h.node.Disconnect(
		user,
		centrifuge.WithDisconnect(disconnect),
		centrifuge.WithDisconnectClient(cmd.Client),
		centrifuge.WithDisconnectClientWhitelist(cmd.Whitelist))
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error disconnecting user", map[string]interface{}{"user": cmd.User, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}
	return resp
}

// Presence returns response with presence information for channel.
func (h *Executor) Presence(_ context.Context, cmd *PresenceRequest) *PresenceResponse {
	defer observe(time.Now(), h.protocol, "presence")

	resp := &PresenceResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if !chOpts.Presence {
		resp.Error = ErrorNotAvailable
		return resp
	}

	presence, err := h.node.Presence(ch)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling presence", map[string]interface{}{"error": err.Error()}))
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
	defer observe(time.Now(), h.protocol, "presence_stats")

	resp := &PresenceStatsResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if !chOpts.Presence {
		resp.Error = ErrorNotAvailable
		return resp
	}

	stats, err := h.node.PresenceStats(cmd.Channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling presence stats", map[string]interface{}{"error": err.Error()}))
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
	defer observe(time.Now(), h.protocol, "history")

	resp := &HistoryResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
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

	history, err := h.node.History(ch, centrifuge.WithLimit(int(cmd.Limit)), centrifuge.WithSince(sp))
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling history", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}

	pubs := history.Publications
	offset := history.Offset
	epoch := history.Epoch

	if cmd.Since != nil {
		sinceEpoch := cmd.Since.Epoch
		sinceOffset := cmd.Since.Offset
		epochOK := sinceEpoch == "" || sinceEpoch == epoch
		offsetOK := cmd.Limit <= 0 || sinceOffset == offset || (sinceOffset < offset && (len(pubs) > 0 && pubs[0].Offset == sinceOffset+1))
		if !epochOK || !offsetOK {
			resp.Error = ErrorUnrecoverablePosition
			return resp
		}
	}

	apiPubs := make([]*Publication, len(history.Publications))

	for i, pub := range history.Publications {
		apiPub := &Publication{
			Data:   Raw(pub.Data),
			Offset: pub.Offset,
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
	defer observe(time.Now(), h.protocol, "history_remove")

	resp := &HistoryRemoveResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, found, err := h.ruleContainer.ChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryTTL <= 0 {
		resp.Error = ErrorNotAvailable
		return resp
	}

	err = h.node.RemoveHistory(ch)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling history remove", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}

	return resp
}

// Info returns information about running nodes.
func (h *Executor) Info(_ context.Context, _ *InfoRequest) *InfoResponse {
	defer observe(time.Now(), h.protocol, "info")

	resp := &InfoResponse{}

	info, err := h.node.Info()
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling info", map[string]interface{}{"error": err.Error()}))
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
			NumChannels: nd.NumChannels,
			Uptime:      nd.Uptime,
			System:      nil,
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
	defer observe(started, h.protocol, "rpc")

	resp := &RPCResponse{}

	if cmd.Method == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	handler, ok := h.rpcExtension[cmd.Method]
	if !ok {
		resp.Error = ErrorMethodNotFound
		return resp
	}

	defer observeRPC(started, h.protocol, cmd.Method)

	data, err := handler(ctx, cmd.Params)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error sending rpc", map[string]interface{}{"error": err.Error()}))
		resp.Error = toAPIErr(err)
		return resp
	}

	resp.Result = &RPCResult{
		Data: data,
	}

	return resp
}

func toAPIErr(err error) *Error {
	if apiErr, ok := err.(*Error); ok {
		return apiErr
	}
	return ErrorInternal
}
