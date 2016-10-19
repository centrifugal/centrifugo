package apiv1

import (
	"encoding/json"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

type APIOptions struct{}

// APICmd builds API command and dispatches it into correct handler method.
func APICmd(app *node.Node, cmd proto.ApiCommand, opts *APIOptions) (proto.Response, error) {

	var err error
	var resp proto.Response

	method := cmd.Method
	params := cmd.Params

	switch method {
	case "publish":
		var cmd proto.PublishAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, proto.ErrInvalidMessage
		}
		resp, err = PublishCmd(app, &cmd)
	case "broadcast":
		var cmd proto.BroadcastAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, proto.ErrInvalidMessage
		}
		resp, err = BroadcastCmd(app, &cmd)
	case "unsubscribe":
		var cmd proto.UnsubscribeAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, proto.ErrInvalidMessage
		}
		resp, err = UnsubcribeCmd(app, &cmd)
	case "disconnect":
		var cmd proto.DisconnectAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, proto.ErrInvalidMessage
		}
		resp, err = DisconnectCmd(app, &cmd)
	case "presence":
		var cmd proto.PresenceAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, proto.ErrInvalidMessage
		}
		resp, err = PresenceCmd(app, &cmd)
	case "history":
		var cmd proto.HistoryAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, proto.ErrInvalidMessage
		}
		resp, err = HistoryCmd(app, &cmd)
	case "channels":
		resp, err = ChannelsCmd(app)
	case "stats":
		resp, err = StatsCmd(app)
	case "node":
		resp, err = NodeCmd(app)
	default:
		return nil, proto.ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	resp.SetUID(cmd.UID)

	return resp, nil
}

// PublishCmd publishes data into channel.
func PublishCmd(app *node.Node, cmd *proto.PublishAPICommand) (proto.Response, error) {
	channel := cmd.Channel
	data := cmd.Data
	err := app.Publish(channel, data, cmd.Client, nil)
	resp := proto.NewAPIPublishResponse()
	if err != nil {
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// BroadcastCmd publishes data into multiple channels.
func BroadcastCmd(app *node.Node, cmd *proto.BroadcastAPICommand) (proto.Response, error) {
	resp := proto.NewAPIBroadcastResponse()
	channels := cmd.Channels
	data := cmd.Data
	if len(channels) == 0 {
		logger.ERROR.Println("channels required for broadcast")
		resp.SetErr(proto.ResponseError{proto.ErrInvalidMessage, proto.ErrorAdviceFix})
		return resp, nil
	}
	errs := make([]<-chan error, len(channels))
	for i, channel := range channels {
		errs[i] = app.PublishAsync(channel, data, cmd.Client, nil)
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
		resp.SetErr(proto.ResponseError{firstErr, proto.ErrorAdviceNone})
	}
	return resp, nil
}

// UnsubscribeCmd unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes.
func UnsubcribeCmd(app *node.Node, cmd *proto.UnsubscribeAPICommand) (proto.Response, error) {
	resp := proto.NewAPIUnsubscribeResponse()
	channel := cmd.Channel
	user := cmd.User
	err := app.Unsubscribe(user, channel)
	if err != nil {
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// DisconnectCmd disconnects user by its ID and sends disconnect
// control message to other nodes so they could also disconnect this user.
func DisconnectCmd(app *node.Node, cmd *proto.DisconnectAPICommand) (proto.Response, error) {
	resp := proto.NewAPIDisconnectResponse()
	user := cmd.User
	err := app.Disconnect(user)
	if err != nil {
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// PresenceCmd returns response with presense information for channel.
func PresenceCmd(app *node.Node, cmd *proto.PresenceAPICommand) (proto.Response, error) {
	channel := cmd.Channel
	body := proto.PresenceBody{
		Channel: channel,
	}
	presence, err := app.Presence(channel)
	if err != nil {
		resp := proto.NewAPIPresenceResponse(body)
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceNone})
		return resp, nil
	}
	body.Data = presence
	return proto.NewAPIPresenceResponse(body), nil
}

// HistoryCmd returns response with history information for channel.
func HistoryCmd(app *node.Node, cmd *proto.HistoryAPICommand) (proto.Response, error) {
	channel := cmd.Channel
	body := proto.HistoryBody{
		Channel: channel,
	}
	history, err := app.History(channel)
	if err != nil {
		resp := proto.NewAPIHistoryResponse(body)
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceNone})
		return resp, nil
	}
	body.Data = history
	return proto.NewAPIHistoryResponse(body), nil
}

// ChannelsCmd returns active channels.
func ChannelsCmd(app *node.Node) (proto.Response, error) {
	body := proto.ChannelsBody{}
	channels, err := app.Channels()
	if err != nil {
		logger.ERROR.Println(err)
		resp := proto.NewAPIChannelsResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrInternalServerError, proto.ErrorAdviceNone})
		return resp, nil
	}
	body.Data = channels
	return proto.NewAPIChannelsResponse(body), nil
}

// StatsCmd returns active node stats.
func StatsCmd(app *node.Node) (proto.Response, error) {
	body := proto.StatsBody{}
	body.Data = app.Stats()
	return proto.NewAPIStatsResponse(body), nil
}

// NodeCmd returns simple counter metrics which update in real time for the current node only.
func NodeCmd(app *node.Node) (proto.Response, error) {
	body := proto.NodeBody{}
	body.Data = app.Node()
	return proto.NewAPINodeResponse(body), nil
}
