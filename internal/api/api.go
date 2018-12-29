package api

import (
	"context"
	"time"

	"github.com/centrifugal/centrifuge"
)

// apiExecutor can run API methods.
type apiExecutor struct {
	node     *centrifuge.Node
	protocol string
}

func newAPIExecutor(n *centrifuge.Node, protocol string) *apiExecutor {
	return &apiExecutor{
		node:     n,
		protocol: protocol,
	}
}

// Publish publishes data into channel.
func (h *apiExecutor) Publish(ctx context.Context, cmd *PublishRequest) *PublishResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "publish").Observe(time.Since(started).Seconds())
	}(time.Now())

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

	_, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	pub := &centrifuge.Publication{
		Data: centrifuge.Raw(cmd.Data),
	}
	if cmd.UID != "" {
		pub.UID = cmd.UID
	}

	err := <-h.node.PublishAsync(cmd.Channel, pub)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error publishing message in engine", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}
	return resp
}

// Broadcast publishes the same data into many channels.
func (h *apiExecutor) Broadcast(ctx context.Context, cmd *BroadcastRequest) *BroadcastResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "broadcast").Observe(time.Since(started).Seconds())
	}(time.Now())

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

	errs := make([]<-chan error, len(channels))

	for i, ch := range channels {

		if ch == "" {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "channel can not be blank in broadcast", nil))
			resp.Error = ErrorBadRequest
			return resp
		}

		_, ok := h.node.ChannelOpts(ch)
		if !ok {
			h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "can't find namespace for channel", map[string]interface{}{"channel": ch}))
			resp.Error = ErrorNamespaceNotFound
			return resp
		}

		pub := &centrifuge.Publication{
			Data: centrifuge.Raw(cmd.Data),
		}
		if cmd.UID != "" {
			pub.UID = cmd.UID
		}
		errs[i] = h.node.PublishAsync(ch, pub)
	}

	var firstErr error
	for i := range errs {
		err := <-errs[i]
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
func (h *apiExecutor) Unsubscribe(ctx context.Context, cmd *UnsubscribeRequest) *UnsubscribeResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "unsubscribe").Observe(time.Since(started).Seconds())
	}(time.Now())

	resp := &UnsubscribeResponse{}

	user := cmd.User
	channel := cmd.Channel

	if user == "" {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "user required for unsubscribe", map[string]interface{}{"channel": channel, "user": user}))
		resp.Error = ErrorBadRequest
		return resp
	}

	if channel != "" {
		_, ok := h.node.ChannelOpts(channel)
		if !ok {
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
func (h *apiExecutor) Disconnect(ctx context.Context, cmd *DisconnectRequest) *DisconnectResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "disconnect").Observe(time.Since(started).Seconds())
	}(time.Now())

	resp := &DisconnectResponse{}

	user := cmd.User
	if user == "" {
		// h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "user required for disconnect", map[string]interface{}{}))
		resp.Error = ErrorBadRequest
		return resp
	}

	err := h.node.Disconnect(user, false)
	if err != nil {
		// h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error disconnecting user", map[string]interface{}{"user": cmd.User, "error": errError()}))
		resp.Error = ErrorInternal
		return resp
	}
	return resp
}

// Presence returns response with presence information for channel.
func (h *apiExecutor) Presence(ctx context.Context, cmd *PresenceRequest) *PresenceResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "presence").Observe(time.Since(started).Seconds())
	}(time.Now())

	resp := &PresenceResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
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
func (h *apiExecutor) PresenceStats(ctx context.Context, cmd *PresenceStatsRequest) *PresenceStatsResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "presence_stats").Observe(time.Since(started).Seconds())
	}(time.Now())

	resp := &PresenceStatsResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
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
func (h *apiExecutor) History(ctx context.Context, cmd *HistoryRequest) *HistoryResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "history").Observe(time.Since(started).Seconds())
	}(time.Now())

	resp := &HistoryResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Error = ErrorNotAvailable
		return resp
	}

	history, err := h.node.History(ch)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling history", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}

	apiPubs := make([]*Publication, len(history))

	for i, pub := range history {
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
func (h *apiExecutor) HistoryRemove(ctx context.Context, cmd *HistoryRemoveRequest) *HistoryRemoveResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "history_remove").Observe(time.Since(started).Seconds())
	}(time.Now())

	resp := &HistoryRemoveResponse{}

	ch := cmd.Channel

	if ch == "" {
		resp.Error = ErrorBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Error = ErrorNotAvailable
		return resp
	}

	err := h.node.RemoveHistory(ch)
	if err != nil {
		h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling history remove", map[string]interface{}{"error": err.Error()}))
		resp.Error = ErrorInternal
		return resp
	}

	return resp
}

// Channels returns active channels.
func (h *apiExecutor) Channels(ctx context.Context, cmd *ChannelsRequest) *ChannelsResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "channels").Observe(time.Since(started).Seconds())
	}(time.Now())

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
func (h *apiExecutor) Info(ctx context.Context, cmd *InfoRequest) *InfoResponse {
	defer func(started time.Time) {
		apiCommandDurationSummary.WithLabelValues(h.protocol, "info").Observe(time.Since(started).Seconds())
	}(time.Now())

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
