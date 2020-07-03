package api

import (
	"context"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/internal/rule"

	"github.com/centrifugal/centrifuge"
)

// apiExecutor can run API methods.
type apiExecutor struct {
	node          *centrifuge.Node
	ruleContainer *rule.ChannelRuleContainer
	protocol      string
}

func newAPIExecutor(n *centrifuge.Node, ruleContainer *rule.ChannelRuleContainer, protocol string) *apiExecutor {
	return &apiExecutor{
		node:          n,
		ruleContainer: ruleContainer,
		protocol:      protocol,
	}
}

// Publish publishes data into channel.
func (h *apiExecutor) Publish(_ context.Context, cmd *PublishRequest) *PublishResponse {
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

	_, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	_, err = h.node.Publish(cmd.Channel, cmd.Data)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error publishing message in engine", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}
	return resp
}

// Broadcast publishes the same data into many channels.
func (h *apiExecutor) Broadcast(_ context.Context, cmd *BroadcastRequest) *BroadcastResponse {
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

		_, err := h.ruleContainer.NamespacedChannelOptions(ch)
		if err != nil {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "can't find namespace for channel", map[string]interface{}{"channel": ch}))
			resp.Error = ErrorNamespaceNotFound
			return resp
		}

		wg.Add(1)
		go func(i int, ch string) {
			_, err := h.node.Publish(ch, data)
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
func (h *apiExecutor) Unsubscribe(_ context.Context, cmd *UnsubscribeRequest) *UnsubscribeResponse {
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
		_, err := h.ruleContainer.NamespacedChannelOptions(channel)
		if err != nil {
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
func (h *apiExecutor) Disconnect(_ context.Context, cmd *DisconnectRequest) *DisconnectResponse {
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
func (h *apiExecutor) Presence(_ context.Context, cmd *PresenceRequest) *PresenceResponse {
	defer observe(time.Now(), h.protocol, "presence")

	resp := &PresenceResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
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

	apiPresence := make(map[string]*ClientInfo, len(presence))
	for k, v := range presence {
		apiPresence[k] = &ClientInfo{
			User:     v.User,
			Client:   v.Client,
			ConnInfo: Raw(v.ConnInfo),
			ChanInfo: Raw(v.ChanInfo),
		}
	}

	resp.Result = &PresenceResult{
		Presence: apiPresence,
	}
	return resp
}

// PresenceStats returns response with presence stats information for channel.
func (h *apiExecutor) PresenceStats(_ context.Context, cmd *PresenceStatsRequest) *PresenceStatsResponse {
	defer observe(time.Now(), h.protocol, "presence_stats")

	resp := &PresenceStatsResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
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
func (h *apiExecutor) History(_ context.Context, cmd *HistoryRequest) *HistoryResponse {
	defer observe(time.Now(), h.protocol, "history")

	resp := &HistoryResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Error = ErrorNotAvailable
		return resp
	}

	history, err := h.node.History(ch, centrifuge.WithNoLimit())
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling history", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}

	apiPubs := make([]*Publication, len(history.Publications))

	for i, pub := range history.Publications {
		apiPub := &Publication{
			UID:  pub.UID,
			Data: Raw(pub.Data),
		}
		if pub.Info != nil {
			apiPub.Info = &ClientInfo{
				User:     pub.Info.User,
				Client:   pub.Info.Client,
				ConnInfo: Raw(pub.Info.ConnInfo),
				ChanInfo: Raw(pub.Info.ChanInfo),
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
func (h *apiExecutor) HistoryRemove(_ context.Context, cmd *HistoryRemoveRequest) *HistoryRemoveResponse {
	defer observe(time.Now(), h.protocol, "history_remove")

	resp := &HistoryRemoveResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, err := h.ruleContainer.NamespacedChannelOptions(ch)
	if err != nil {
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
func (h *apiExecutor) Channels(_ context.Context, _ *ChannelsRequest) *ChannelsResponse {
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
func (h *apiExecutor) Info(_ context.Context, _ *InfoRequest) *InfoResponse {
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
