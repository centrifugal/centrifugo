package api

import (
	"context"

	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
	"github.com/centrifugal/centrifugo/lib/proto/apiproto"
)

// Handler ...
type Handler struct {
	node *node.Node
}

// NewHandler ...
func NewHandler(n *node.Node) *Handler {
	return &Handler{
		node: n,
	}
}

// Publish publishes data into channel.
func (h *Handler) Publish(ctx context.Context, cmd *apiproto.PublishRequest) *apiproto.PublishResponse {
	ch := cmd.Channel
	data := cmd.Data

	resp := &apiproto.PublishResponse{}

	if string(ch) == "" || len(data) == 0 {
		logger.ERROR.Printf("channel and data required for publish")
		resp.Error = apiproto.ErrBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = apiproto.ErrNamespaceNotFound
		return resp
	}

	publication := &proto.Publication{
		Data: cmd.Data,
	}

	err := <-h.node.Publish(cmd.Channel, publication, &chOpts)
	if err != nil {
		logger.ERROR.Printf("error publishing message: %v", err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}
	return resp
}

// Broadcast publishes data into multiple channels.
func (h *Handler) Broadcast(ctx context.Context, cmd *apiproto.BroadcastRequest) *apiproto.BroadcastResponse {

	resp := &apiproto.BroadcastResponse{}

	channels := cmd.Channels
	data := cmd.Data

	if len(channels) == 0 {
		logger.ERROR.Println("channels required for broadcast")
		resp.Error = apiproto.ErrBadRequest
		return resp
	}

	if len(data) == 0 {
		logger.ERROR.Println("data required for broadcast")
		resp.Error = apiproto.ErrBadRequest
		return resp
	}

	errs := make([]<-chan error, len(channels))

	for i, ch := range channels {

		if string(ch) == "" {
			logger.ERROR.Println("channel can not be blank in broadcast")
			resp.Error = apiproto.ErrBadRequest
			return resp
		}

		chOpts, ok := h.node.ChannelOpts(ch)
		if !ok {
			logger.ERROR.Panicf("can't find namespace for channel %s", ch)
			resp.Error = apiproto.ErrNamespaceNotFound
		}

		publication := &proto.Publication{
			Data: cmd.Data,
		}
		errs[i] = h.node.Publish(ch, publication, &chOpts)
	}

	var firstErr error
	for i := range errs {
		err := <-errs[i]
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			logger.ERROR.Printf("Error publishing into channel %s: %v", string(channels[i]), err.Error())
		}
	}
	if firstErr != nil {
		logger.ERROR.Printf("error broadcasting: %v", firstErr)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}
	return resp
}

// Unsubscribe unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes.
func (h *Handler) Unsubscribe(ctx context.Context, cmd *apiproto.UnsubscribeRequest) *apiproto.UnsubscribeResponse {

	resp := &apiproto.UnsubscribeResponse{}

	user := cmd.User
	channel := cmd.Channel

	err := h.node.Unsubscribe(user, channel)
	if err != nil {
		logger.ERROR.Printf("error unsubscribing user %s from channel %s: %v", user, channel, err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}
	return resp
}

// Disconnect disconnects user by its ID and sends disconnect
// control message to other nodes so they could also disconnect this user.
func (h *Handler) Disconnect(ctx context.Context, cmd *apiproto.DisconnectRequest) *apiproto.DisconnectResponse {

	resp := &apiproto.DisconnectResponse{}

	user := cmd.User

	err := h.node.Disconnect(user, false)
	if err != nil {
		logger.ERROR.Printf("error disconnecting user with ID %s: %v", cmd.User, err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}
	return resp
}

// Presence returns response with presence information for channel.
func (h *Handler) Presence(ctx context.Context, cmd *apiproto.PresenceRequest) *apiproto.PresenceResponse {

	resp := &apiproto.PresenceResponse{}

	ch := cmd.Channel

	if string(ch) == "" {
		resp.Error = apiproto.ErrBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = apiproto.ErrNamespaceNotFound
		return resp
	}

	if !chOpts.Presence {
		resp.Error = apiproto.ErrNotAvailable
		return resp
	}

	presence, err := h.node.Presence(ch)
	if err != nil {
		logger.ERROR.Printf("error calling presence: %v", err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}

	apiPresence := make(map[string]*apiproto.ClientInfo, len(presence))
	for k, v := range presence {
		apiPresence[k] = (*apiproto.ClientInfo)(v)
	}

	resp.Result = &apiproto.PresenceResult{
		Presence: apiPresence,
	}
	return resp
}

// PresenceStats returns response with presence stats information for channel.
func (h *Handler) PresenceStats(ctx context.Context, cmd *apiproto.PresenceStatsRequest) *apiproto.PresenceStatsResponse {

	resp := &apiproto.PresenceStatsResponse{}

	ch := cmd.Channel

	if string(ch) == "" {
		resp.Error = apiproto.ErrBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = apiproto.ErrNamespaceNotFound
		return resp
	}

	if !chOpts.Presence || !chOpts.PresenceStats {
		resp.Error = apiproto.ErrNotAvailable
		return resp
	}

	presence, err := h.node.Presence(cmd.Channel)
	if err != nil {
		logger.ERROR.Printf("error calling presence: %v", err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}

	numClients := len(presence)
	numUsers := 0
	uniqueUsers := map[string]struct{}{}

	for _, info := range presence {
		userID := info.User
		if _, ok := uniqueUsers[userID]; !ok {
			uniqueUsers[userID] = struct{}{}
			numUsers++
		}
	}

	resp.Result = &apiproto.PresenceStatsResult{
		NumClients: uint64(numClients),
		NumUsers:   uint64(numUsers),
	}

	return resp
}

// History returns response with history information for channel.
func (h *Handler) History(ctx context.Context, cmd *apiproto.HistoryRequest) *apiproto.HistoryResponse {

	resp := &apiproto.HistoryResponse{}

	ch := cmd.Channel

	if string(ch) == "" {
		resp.Error = apiproto.ErrBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = apiproto.ErrNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Error = apiproto.ErrNotAvailable
		return resp
	}

	history, err := h.node.History(ch)
	if err != nil {
		logger.ERROR.Printf("error calling history: %v", err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}

	apiPublications := make([]*apiproto.Publication, len(history))

	for i, publication := range history {
		apiPublications[i] = &apiproto.Publication{
			UID:  publication.UID,
			Data: publication.Data,
			Info: (*apiproto.ClientInfo)(publication.Info),
		}
	}

	resp.Result = &apiproto.HistoryResult{
		Publications: apiPublications,
	}
	return resp
}

// Channels returns active channels.
func (h *Handler) Channels(ctx context.Context, cmd *apiproto.ChannelsRequest) *apiproto.ChannelsResponse {

	resp := &apiproto.ChannelsResponse{}

	channels, err := h.node.Channels()
	if err != nil {
		logger.ERROR.Printf("error calling channels: %v", err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}

	resp.Result = &apiproto.ChannelsResult{
		Channels: channels,
	}
	return resp
}

// Info returns active node info.
func (h *Handler) Info(ctx context.Context, cmd *apiproto.InfoRequest) *apiproto.InfoResponse {

	resp := &apiproto.InfoResponse{}

	info, err := h.node.Info()
	if err != nil {
		logger.ERROR.Printf("error calling stats: %v", err)
		resp.Error = apiproto.ErrInternalServerError
		return resp
	}

	resp.Result = info
	return resp
}
