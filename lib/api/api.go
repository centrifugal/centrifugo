package api

import (
	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
	apiproto "github.com/centrifugal/centrifugo/lib/proto/api"
)

// RequestHandler ...
type RequestHandler struct {
	node    *node.Node
	decoder apiproto.Decoder
	encoder apiproto.Encoder
}

// NewJSONRequestHandler ...
func NewJSONRequestHandler(n *node.Node) *RequestHandler {
	return &RequestHandler{
		node:    n,
		decoder: apiproto.NewJSONDecoder(),
		encoder: apiproto.NewJSONEncoder(),
	}
}

// NewProtobufRequestHandler ...
func NewProtobufRequestHandler(n *node.Node) *RequestHandler {
	return &RequestHandler{
		node:    n,
		decoder: apiproto.NewProtobufDecoder(),
		encoder: apiproto.NewProtobufEncoder(),
	}
}

// Handle extracts commands from data, run them sequentially and returns
// encoded response.
func (h *RequestHandler) Handle(data []byte) ([]byte, error) {

	request, err := h.decoder.DecodeRequest(data)
	if err != nil {
		return nil, err
	}

	replies := make([]*apiproto.Reply, len(request.Commands))

	if len(request.Commands) == 0 {
		return nil, proto.ErrBadRequest
	}

	for i, command := range request.Commands {
		rep, err := h.handleCommand(command)
		if err != nil {
			return nil, err
		}
		replies[i] = rep
	}

	response := &apiproto.Response{
		Replies: replies,
	}

	resp, err := h.encoder.EncodeResponse(response)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (h *RequestHandler) handleCommand(cmd *apiproto.Command) (*apiproto.Reply, error) {

	var err error

	method := cmd.Method
	params := cmd.Params

	var replyRes proto.Raw
	var replyErr *proto.Error

	rep := &apiproto.Reply{
		ID: cmd.ID,
	}

	if method == "" {
		rep.Error = proto.ErrBadRequest
		return rep, nil
	}

	switch method {
	case "publish":
		cmd, err := h.decoder.DecodePublish(params)
		if err != nil {
			logger.ERROR.Printf("error decoding: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		var res *apiproto.PublishResult
		res, replyErr = CallPublish(h.node, cmd)
		if replyErr == nil {
			replyRes, err = h.encoder.EncodePublishResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "broadcast":
		cmd, err := h.decoder.DecodeBroadcast(params)
		if err != nil {
			logger.ERROR.Printf("error decoding: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		var res *apiproto.BroadcastResult
		res, replyErr = CallBroadcast(h.node, cmd)
		if replyErr == nil {
			replyRes, err = h.encoder.EncodeBroadcastResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "unsubscribe":
		cmd, err := h.decoder.DecodeUnsubscribe(params)
		if err != nil {
			logger.ERROR.Printf("error decoding: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		var res *apiproto.UnsubscribeResult
		res, replyErr = CallUnsubscribe(h.node, cmd)
		if replyErr == nil {
			replyRes, err = h.encoder.EncodeUnsubscribeResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "disconnect":
		cmd, err := h.decoder.DecodeDisconnect(params)
		if err != nil {
			logger.ERROR.Printf("error decoding: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		var res *apiproto.DisconnectResult
		res, replyErr = CallDisconnect(h.node, cmd)
		if replyErr == nil {
			replyRes, err = h.encoder.EncodeDisconnectResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "presence":
		cmd, err := h.decoder.DecodePresence(params)
		if err != nil {
			logger.ERROR.Printf("error decoding: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		var res *apiproto.PresenceResult
		res, replyErr = CallPresence(h.node, cmd)
		if replyErr == nil {
			replyRes, err = h.encoder.EncodePresenceResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "presence_stats":
		cmd, err := h.decoder.DecodePresenceStats(params)
		if err != nil {
			logger.ERROR.Printf("error decoding: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		var res *apiproto.PresenceStatsResult
		res, replyErr = CallPresenceStats(h.node, cmd)
		if replyErr == nil {
			replyRes, err = h.encoder.EncodePresenceStatsResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "history":
		cmd, err := h.decoder.DecodeHistory(params)
		if err != nil {
			logger.ERROR.Printf("error decoding: %v", err)
			rep.Error = proto.ErrBadRequest
			return rep, nil
		}
		var res *apiproto.HistoryResult
		res, replyErr = CallHistory(h.node, cmd)
		if replyErr == nil {
			replyRes, err = h.encoder.EncodeHistoryResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "channels":
		res, replyErr := CallChannels(h.node, &apiproto.Channels{})
		if replyErr == nil {
			replyRes, err = h.encoder.EncodeChannelsResult(res)
			if err != nil {
				return nil, err
			}
		}
	case "info":
		res, replyErr := CallInfo(h.node, &apiproto.Info{})
		if replyErr == nil {
			replyRes, err = h.encoder.EncodeInfoResult(res)
			if err != nil {
				return nil, err
			}
		}
	default:
		replyErr = proto.ErrMethodNotFound
	}

	rep.Result = replyRes
	rep.Error = replyErr

	return rep, nil
}

// CallPublish publishes data into channel.
func CallPublish(n *node.Node, cmd *apiproto.Publish) (*apiproto.PublishResult, *proto.Error) {
	ch := cmd.Channel
	data := cmd.Data

	if string(ch) == "" || len(data) == 0 {
		logger.ERROR.Printf("channel and data required for publish")
		return nil, proto.ErrBadRequest
	}

	res := &apiproto.PublishResult{}

	chOpts, ok := n.ChannelOpts(ch)
	if !ok {
		return nil, proto.ErrNamespaceNotFound
	}

	publication := &proto.Publication{
		Data: cmd.Data,
	}

	err := <-n.Publish(cmd.Channel, publication, &chOpts)
	if err != nil {
		logger.ERROR.Printf("error publishing message: %v", err)
		return nil, proto.ErrInternalServerError
	}
	return res, nil
}

func makeProtoErrChan(err *proto.Error) <-chan *proto.Error {
	ret := make(chan *proto.Error, 1)
	ret <- err
	return ret
}

// CallPublishAsync publishes data into channel without waiting for response.
func CallPublishAsync(n *node.Node, cmd *apiproto.Publish) <-chan *proto.Error {
	ch := cmd.Channel
	data := cmd.Data

	if string(ch) == "" || len(data) == 0 {
		logger.ERROR.Printf("channel and data required for publish")
		return makeProtoErrChan(proto.ErrBadRequest)
	}

	chOpts, ok := n.ChannelOpts(ch)
	if !ok {
		return makeProtoErrChan(proto.ErrNamespaceNotFound)
	}

	publication := &proto.Publication{
		Data: cmd.Data,
	}

	n.Publish(cmd.Channel, publication, &chOpts)

	return makeProtoErrChan(nil)
}

// CallBroadcast publishes data into multiple channels.
func CallBroadcast(n *node.Node, cmd *apiproto.Broadcast) (*apiproto.BroadcastResult, *proto.Error) {

	res := &apiproto.BroadcastResult{}

	channels := cmd.Channels
	data := cmd.Data

	if len(channels) == 0 {
		logger.ERROR.Println("channels required for broadcast")
		return nil, proto.ErrBadRequest
	}

	if len(data) == 0 {
		logger.ERROR.Println("data required for broadcast")
		return res, proto.ErrBadRequest
	}

	errs := make([]<-chan error, len(channels))

	for i, ch := range channels {

		if string(ch) == "" {
			logger.ERROR.Println("channel can not be blank in broadcast")
			return res, proto.ErrBadRequest
		}

		chOpts, ok := n.ChannelOpts(ch)
		if !ok {
			return nil, proto.ErrNamespaceNotFound
		}

		publication := &proto.Publication{
			Data: cmd.Data,
		}
		errs[i] = n.Publish(ch, publication, &chOpts)
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
		return nil, proto.ErrInternalServerError
	}
	return res, nil
}

// CallBroadcastAsync publishes data into multiple channels without waiting for response.
func CallBroadcastAsync(n *node.Node, cmd *apiproto.Broadcast) <-chan *proto.Error {

	channels := cmd.Channels
	data := cmd.Data

	if len(channels) == 0 {
		logger.ERROR.Println("channels required for broadcast")
		return makeProtoErrChan(proto.ErrBadRequest)
	}

	if len(data) == 0 {
		logger.ERROR.Println("data required for broadcast")
		return makeProtoErrChan(proto.ErrBadRequest)
	}

	for _, ch := range channels {

		if string(ch) == "" {
			logger.ERROR.Println("channel can not be blank in broadcast")
			return makeProtoErrChan(proto.ErrBadRequest)
		}

		chOpts, ok := n.ChannelOpts(ch)
		if !ok {
			return makeProtoErrChan(proto.ErrNamespaceNotFound)
		}

		publication := &proto.Publication{
			Data: cmd.Data,
		}
		n.Publish(ch, publication, &chOpts)
	}
	return makeProtoErrChan(nil)
}

// CallUnsubscribe unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes.
func CallUnsubscribe(n *node.Node, cmd *apiproto.Unsubscribe) (*apiproto.UnsubscribeResult, *proto.Error) {

	res := &apiproto.UnsubscribeResult{}

	user := cmd.User
	channel := cmd.Channel

	err := n.Unsubscribe(user, channel)
	if err != nil {
		logger.ERROR.Printf("error unsubscribing: %v", err)
		return nil, proto.ErrInternalServerError
	}
	return res, nil
}

// CallDisconnect disconnects user by its ID and sends disconnect
// control message to other nodes so they could also disconnect this user.
func CallDisconnect(n *node.Node, cmd *apiproto.Disconnect) (*apiproto.DisconnectResult, *proto.Error) {

	res := &apiproto.DisconnectResult{}

	user := cmd.User

	err := n.Disconnect(user, false)
	if err != nil {
		logger.ERROR.Printf("error disconnecting: %v", err)
		return nil, proto.ErrInternalServerError
	}
	return res, nil
}

// CallPresence returns response with presence information for channel.
func CallPresence(n *node.Node, cmd *apiproto.Presence) (*apiproto.PresenceResult, *proto.Error) {

	channel := cmd.Channel

	presence, err := n.Presence(channel)
	if err != nil {
		logger.ERROR.Printf("error calling presence: %v", err)
		return nil, proto.ErrInternalServerError
	}

	res := &apiproto.PresenceResult{
		Data: presence,
	}
	return res, nil
}

// CallPresenceStats returns response with presence stats information for channel.
func CallPresenceStats(n *node.Node, cmd *apiproto.PresenceStats) (*apiproto.PresenceStatsResult, *proto.Error) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, proto.ErrBadRequest
	}

	chOpts, ok := n.ChannelOpts(ch)
	if !ok {
		return nil, proto.ErrNamespaceNotFound
	}

	if !chOpts.Presence {
		return nil, proto.ErrNotAvailable
	}

	if !chOpts.PresenceStats {
		return nil, proto.ErrNotAvailable
	}

	presence, err := n.Presence(cmd.Channel)
	if err != nil {
		logger.ERROR.Printf("error calling presence: %v", err)
		return nil, proto.ErrInternalServerError
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

	presenceStats := &apiproto.PresenceStatsResult{
		NumClients: uint64(numClients),
		NumUsers:   uint64(numUsers),
	}
	return presenceStats, nil
}

// CallHistory returns response with history information for channel.
func CallHistory(n *node.Node, cmd *apiproto.History) (*apiproto.HistoryResult, *proto.Error) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, proto.ErrBadRequest
	}

	chOpts, ok := n.ChannelOpts(ch)
	if !ok {
		return nil, proto.ErrNamespaceNotFound
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		return nil, proto.ErrNotAvailable
	}

	history, err := n.History(ch)
	if err != nil {
		logger.ERROR.Printf("error calling history: %v", err)
		return nil, proto.ErrInternalServerError
	}

	res := &apiproto.HistoryResult{
		Data: history,
	}
	return res, nil
}

// CallChannels returns active channels.
func CallChannels(n *node.Node, cmd *apiproto.Channels) (*apiproto.ChannelsResult, *proto.Error) {

	channels, err := n.Channels()
	if err != nil {
		logger.ERROR.Printf("error calling channels: %v", err)
		return nil, proto.ErrInternalServerError
	}

	res := &apiproto.ChannelsResult{
		Data: channels,
	}
	return res, nil
}

// CallInfo returns active node info.
func CallInfo(n *node.Node, cmd *apiproto.Info) (*apiproto.InfoResult, *proto.Error) {
	info, err := n.Info()
	if err != nil {
		logger.ERROR.Printf("error calling stats: %v", err)
		return nil, proto.ErrInternalServerError
	}
	return info, nil
}
