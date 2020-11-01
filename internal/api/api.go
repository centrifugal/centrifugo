package api

import (
	"context"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/internal/rule"

	"github.com/centrifugal/centrifuge"
)

// RPCHandler allows to handle custom RPC.
type RPCHandler func(ctx context.Context, params Raw) (Raw, error)

// Executor can run API methods.
type Executor struct {
	node          *centrifuge.Node
	ruleContainer *rule.ChannelRuleContainer
	protocol      string
	rpcExtension  map[string]RPCHandler
}

// NewExecutor ...
func NewExecutor(n *centrifuge.Node, ruleContainer *rule.ChannelRuleContainer, protocol string) *Executor {
	return &Executor{
		node:          n,
		ruleContainer: ruleContainer,
		protocol:      protocol,
		rpcExtension:  make(map[string]RPCHandler),
	}
}

// SetRPCExtension ...
func (h Executor) SetRPCExtension(method string, handler RPCHandler) {
	h.rpcExtension[method] = handler
}

// Publish publishes data into channel.
func (h *Executor) Publish(_ context.Context, cmd *PublishRequest) *PublishResponse {
	defer observe(time.Now(), h.protocol, "publish")

	ch := cmd.Channel
	data := cmd.Data

	resp := &PublishResponse{}

	if ch == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel required for publish", nil))
		resp.Error = ErrorBadRequest
		return resp
	}

	if len(data) == 0 {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "data required for publish", nil))
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	_, err = h.node.Publish(
		cmd.Channel, cmd.Data,
		centrifuge.WithHistory(chOpts.HistorySize, time.Duration(chOpts.HistoryLifetime)*time.Second),
	)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error publishing message in engine", map[string]interface{}{"error": err.Error(), "channel": cmd.Channel}))
		resp.Error = ErrorInternal
		return resp
	}
	return resp
}

// Broadcast publishes the same data into many channels.
func (h *Executor) Broadcast(_ context.Context, cmd *BroadcastRequest) *BroadcastResponse {
	defer observe(time.Now(), h.protocol, "broadcast")

	resp := &BroadcastResponse{}

	channels := cmd.Channels
	data := cmd.Data

	if len(channels) == 0 {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channels required for broadcast", nil))
		resp.Error = ErrorBadRequest
		return resp
	}

	if len(data) == 0 {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "data required for broadcast", nil))
		resp.Error = ErrorBadRequest
		return resp
	}

	errs := make([]error, len(channels))

	var wg sync.WaitGroup

	for i, ch := range channels {

		if ch == "" {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel can not be blank in broadcast", nil))
			resp.Error = ErrorBadRequest
			return resp
		}

		chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error getting options for channel", map[string]interface{}{"channel": ch, "error": err.Error()}))
			resp.Error = ErrorInternal
			return resp
		}
		if !found {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "can't find namespace for channel", map[string]interface{}{"channel": ch}))
			resp.Error = ErrorNamespaceNotFound
			return resp
		}

		wg.Add(1)
		go func(i int, ch string) {
			_, err := h.node.Publish(
				ch, data,
				centrifuge.WithHistory(chOpts.HistorySize, time.Duration(chOpts.HistoryLifetime)*time.Second),
			)
			errs[i] = err
			wg.Done()
		}(i, ch)
	}
	wg.Wait()

	var firstErr error
	for i := range errs {
		err := errs[i]
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error publishing into channel", map[string]interface{}{"channel": channels[i], "error": err.Error()}))
		}
	}
	if firstErr != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error broadcasting data", map[string]interface{}{"error": firstErr.Error()}))
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
		_, found, err := h.ruleContainer.NamespacedChannelOptions(channel)
		if err != nil {
			resp.Error = ErrorInternal
			return resp
		}
		if !found {
			resp.Error = ErrorNamespaceNotFound
			return resp
		}
	}

	err := h.node.Unsubscribe(user, channel)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error unsubscribing user from channel", map[string]interface{}{"channel": channel, "user": user, "error": err.Error()}))
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

	err := h.node.Disconnect(user)
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

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
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

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
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

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Error = ErrorNotAvailable
		return resp
	}

	history, err := h.node.History(ch, centrifuge.WithLimit(centrifuge.NoLimit))
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling history", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}

	apiPubs := make([]*Publication, len(history.Publications))

	for i, pub := range history.Publications {
		apiPub := &Publication{
			Data: Raw(pub.Data),
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

	chOpts, found, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorInternal
		return resp
	}
	if !found {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
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

// Channels returns active channels.
func (h *Executor) Channels(_ context.Context, _ *ChannelsRequest) *ChannelsResponse {
	defer observe(time.Now(), h.protocol, "channels")

	resp := &ChannelsResponse{}

	channels, err := h.node.Channels()
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling channels", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}

	resp.Result = &ChannelsResult{
		Channels: channels,
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
			UID:         nd.UID,
			Version:     nd.Version,
			Name:        nd.Name,
			NumClients:  nd.NumClients,
			NumUsers:    nd.NumUsers,
			NumChannels: nd.NumChannels,
			Uptime:      nd.Uptime,
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

// RPC ...
func (h *Executor) RPC(ctx context.Context, cmd *RPCRequest) *RPCResponse {
	defer observe(time.Now(), h.protocol, "history_remove")

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

	data, err := handler(ctx, cmd.Params)
	if err != nil {
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
