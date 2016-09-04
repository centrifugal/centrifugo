package node

import (
	"encoding/json"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

// APICmd builds API command and dispatches it into correct handler method.
func (app *Application) APICmd(cmd proto.ApiCommand) (proto.Response, error) {

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
			return nil, ErrInvalidMessage
		}
		resp, err = app.publishCmd(&cmd)
	case "broadcast":
		var cmd proto.BroadcastAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.broadcastCmd(&cmd)
	case "unsubscribe":
		var cmd proto.UnsubscribeAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.unsubcribeCmd(&cmd)
	case "disconnect":
		var cmd proto.DisconnectAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.disconnectCmd(&cmd)
	case "presence":
		var cmd proto.PresenceAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.presenceCmd(&cmd)
	case "history":
		var cmd proto.HistoryAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.historyCmd(&cmd)
	case "channels":
		resp, err = app.channelsCmd()
	case "stats":
		resp, err = app.statsCmd()
	case "node":
		resp, err = app.nodeCmd()
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	resp.SetUID(cmd.UID)

	return resp, nil
}

// publishCmd publishes data into channel.
func (app *Application) publishCmd(cmd *proto.PublishAPICommand) (proto.Response, error) {
	channel := cmd.Channel
	data := cmd.Data
	err := app.publish(channel, data, cmd.Client, nil, false)
	resp := proto.NewAPIPublishResponse()
	if err != nil {
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// broadcastCmd publishes data into multiple channels.
func (app *Application) broadcastCmd(cmd *proto.BroadcastAPICommand) (proto.Response, error) {
	resp := proto.NewAPIBroadcastResponse()
	channels := cmd.Channels
	data := cmd.Data
	if len(channels) == 0 {
		logger.ERROR.Println("channels required for broadcast")
		resp.SetErr(proto.ResponseError{ErrInvalidMessage, proto.ErrorAdviceNone})
		return resp, nil
	}
	errs := make([]<-chan error, len(channels))
	for i, channel := range channels {
		errs[i] = app.publishAsync(channel, data, cmd.Client, nil, false)
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

// unsubscribeCmd unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes.
func (app *Application) unsubcribeCmd(cmd *proto.UnsubscribeAPICommand) (proto.Response, error) {
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

// disconnectCmd disconnects user by its ID and sends disconnect
// control message to other nodes so they could also disconnect this user.
func (app *Application) disconnectCmd(cmd *proto.DisconnectAPICommand) (proto.Response, error) {
	resp := proto.NewAPIDisconnectResponse()
	user := cmd.User
	err := app.Disconnect(user)
	if err != nil {
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// presenceCmd returns response with presense information for channel.
func (app *Application) presenceCmd(cmd *proto.PresenceAPICommand) (proto.Response, error) {
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

// historyCmd returns response with history information for channel.
func (app *Application) historyCmd(cmd *proto.HistoryAPICommand) (proto.Response, error) {
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

// channelsCmd returns active channels.
func (app *Application) channelsCmd() (proto.Response, error) {
	body := proto.ChannelsBody{}
	channels, err := app.channels()
	if err != nil {
		logger.ERROR.Println(err)
		resp := proto.NewAPIChannelsResponse(body)
		resp.SetErr(proto.ResponseError{ErrInternalServerError, proto.ErrorAdviceNone})
		return resp, nil
	}
	body.Data = channels
	return proto.NewAPIChannelsResponse(body), nil
}

// statsCmd returns active node stats.
func (app *Application) statsCmd() (proto.Response, error) {
	body := proto.StatsBody{}
	body.Data = app.stats()
	return proto.NewAPIStatsResponse(body), nil
}

// nodeCmd returns simple counter metrics which update in real time for the current node only.
func (app *Application) nodeCmd() (proto.Response, error) {
	body := proto.NodeBody{}
	body.Data = app.node()
	return proto.NewAPINodeResponse(body), nil
}
