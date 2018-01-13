package api

import (
	"context"

	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
	apiproto "github.com/centrifugal/centrifugo/lib/proto/api"
)

// Handler ...
type Handler struct {
	node *node.Node
	enc  apiproto.Encoding
}

// NewHandler ...
func NewHandler(n *node.Node, enc apiproto.Encoding) *Handler {
	return &Handler{
		node: n,
		enc:  enc,
	}
}

// Handle extracts commands from data, run them sequentially and returns
// encoded response.
func (h *Handler) Handle(ctx context.Context, cmd *apiproto.Command) (*apiproto.Reply, error) {

	var err error

	method := cmd.Method
	params := cmd.Params

	rep := &apiproto.Reply{
		ID: cmd.ID,
	}

	var replyRes proto.Raw

	if method == "" {
		logger.ERROR.Println("method required in API command")
		rep.Error = proto.ErrBadRequest
		return rep, nil
	}

	decoder := apiproto.GetDecoder(h.enc)
	defer apiproto.PutDecoder(h.enc, decoder)

	encoder := apiproto.GetEncoder(h.enc)
	defer apiproto.PutEncoder(h.enc, encoder)

	switch method {
	case "publish":
		cmd, err := decoder.DecodePublish(params)
		if err != nil {
			logger.ERROR.Printf("error decoding publish params: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		resp := h.Publish(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePublish(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "broadcast":
		cmd, err := decoder.DecodeBroadcast(params)
		if err != nil {
			logger.ERROR.Printf("error decoding broadcast params: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		resp := h.Broadcast(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeBroadcast(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "unsubscribe":
		cmd, err := decoder.DecodeUnsubscribe(params)
		if err != nil {
			logger.ERROR.Printf("error decoding unsubscribe params: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		resp := h.Unsubscribe(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeUnsubscribe(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "disconnect":
		cmd, err := decoder.DecodeDisconnect(params)
		if err != nil {
			logger.ERROR.Printf("error decoding disconnect params: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		resp := h.Disconnect(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeDisconnect(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "presence":
		cmd, err := decoder.DecodePresence(params)
		if err != nil {
			logger.ERROR.Printf("error decoding presence params: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		resp := h.Presence(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePresence(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "presence_stats":
		cmd, err := decoder.DecodePresenceStats(params)
		if err != nil {
			logger.ERROR.Printf("error decoding presence_stats params: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		resp := h.PresenceStats(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodePresenceStats(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "history":
		cmd, err := decoder.DecodeHistory(params)
		if err != nil {
			logger.ERROR.Printf("error decoding history params: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		resp := h.History(ctx, cmd)
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeHistory(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "channels":
		resp := h.Channels(ctx, &apiproto.ChannelsRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeChannels(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	case "info":
		resp := h.Info(ctx, &apiproto.InfoRequest{})
		if resp.Error != nil {
			rep.Error = resp.Error
		} else {
			if resp.Result != nil {
				replyRes, err = encoder.EncodeInfo(resp.Result)
				if err != nil {
					return nil, err
				}
			}
		}
	default:
		rep.Error = proto.ErrMethodNotFound
	}

	if len(replyRes) > 0 {
		rep.Result = (*proto.Raw)(&replyRes)
	}

	return rep, nil
}

// Publish publishes data into channel.
func (h *Handler) Publish(ctx context.Context, cmd *apiproto.PublishRequest) *apiproto.PublishResponse {
	ch := cmd.Channel
	data := cmd.Data

	resp := &apiproto.PublishResponse{}

	if string(ch) == "" || len(data) == 0 {
		logger.ERROR.Printf("channel and data required for publish")
		resp.Error = proto.ErrBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = proto.ErrNamespaceNotFound
		return resp
	}

	publication := &proto.Publication{
		Data: cmd.Data,
	}

	err := <-h.node.Publish(cmd.Channel, publication, &chOpts)
	if err != nil {
		logger.ERROR.Printf("error publishing message: %v", err)
		resp.Error = proto.ErrInternalServerError
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
		resp.Error = proto.ErrBadRequest
		return resp
	}

	if len(data) == 0 {
		logger.ERROR.Println("data required for broadcast")
		resp.Error = proto.ErrBadRequest
		return resp
	}

	errs := make([]<-chan error, len(channels))

	for i, ch := range channels {

		if string(ch) == "" {
			logger.ERROR.Println("channel can not be blank in broadcast")
			resp.Error = proto.ErrBadRequest
			return resp
		}

		chOpts, ok := h.node.ChannelOpts(ch)
		if !ok {
			logger.ERROR.Panicf("can't find namespace for channel %s", ch)
			resp.Error = proto.ErrNamespaceNotFound
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
		resp.Error = proto.ErrInternalServerError
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
		resp.Error = proto.ErrInternalServerError
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
		resp.Error = proto.ErrInternalServerError
		return resp
	}
	return resp
}

// Presence returns response with presence information for channel.
func (h *Handler) Presence(ctx context.Context, cmd *apiproto.PresenceRequest) *apiproto.PresenceResponse {

	resp := &apiproto.PresenceResponse{}

	channel := cmd.Channel

	presence, err := h.node.Presence(channel)
	if err != nil {
		logger.ERROR.Printf("error calling presence: %v", err)
		resp.Error = proto.ErrInternalServerError
		return resp
	}

	resp.Result = &apiproto.PresenceResult{
		Presence: presence,
	}
	return resp
}

// PresenceStats returns response with presence stats information for channel.
func (h *Handler) PresenceStats(ctx context.Context, cmd *apiproto.PresenceStatsRequest) *apiproto.PresenceStatsResponse {

	resp := &apiproto.PresenceStatsResponse{}

	ch := cmd.Channel

	if string(ch) == "" {
		resp.Error = proto.ErrBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = proto.ErrNamespaceNotFound
		return resp
	}

	if !chOpts.Presence || !chOpts.PresenceStats {
		resp.Error = proto.ErrNotAvailable
		return resp
	}

	presence, err := h.node.Presence(cmd.Channel)
	if err != nil {
		logger.ERROR.Printf("error calling presence: %v", err)
		resp.Error = proto.ErrInternalServerError
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
		resp.Error = proto.ErrBadRequest
		return resp
	}

	chOpts, ok := h.node.ChannelOpts(ch)
	if !ok {
		resp.Error = proto.ErrNamespaceNotFound
		return resp
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Error = proto.ErrNotAvailable
		return resp
	}

	history, err := h.node.History(ch)
	if err != nil {
		logger.ERROR.Printf("error calling history: %v", err)
		resp.Error = proto.ErrInternalServerError
		return resp
	}

	resp.Result = &apiproto.HistoryResult{
		Publications: history,
	}
	return resp
}

// Channels returns active channels.
func (h *Handler) Channels(ctx context.Context, cmd *apiproto.ChannelsRequest) *apiproto.ChannelsResponse {

	resp := &apiproto.ChannelsResponse{}

	channels, err := h.node.Channels()
	if err != nil {
		logger.ERROR.Printf("error calling channels: %v", err)
		resp.Error = proto.ErrInternalServerError
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
		resp.Error = proto.ErrInternalServerError
		return resp
	}

	resp.Result = info
	return resp
}
