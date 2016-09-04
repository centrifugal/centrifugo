package libcentrifugo

import (
	"encoding/json"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/commands"
	"github.com/centrifugal/centrifugo/libcentrifugo/response"
)

// apiCmd builds API command and dispatches it into correct handler method.
func (app *Application) ApiCmd(cmd commands.ApiCommand) (response.Response, error) {

	var err error
	var resp response.Response

	method := cmd.Method
	params := cmd.Params

	switch method {
	case "publish":
		var cmd commands.PublishAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.publishCmd(&cmd)
	case "broadcast":
		var cmd commands.BroadcastAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.broadcastCmd(&cmd)
	case "unsubscribe":
		var cmd commands.UnsubscribeAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.unsubcribeCmd(&cmd)
	case "disconnect":
		var cmd commands.DisconnectAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.disconnectCmd(&cmd)
	case "presence":
		var cmd commands.PresenceAPICommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.presenceCmd(&cmd)
	case "history":
		var cmd commands.HistoryAPICommand
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
func (app *Application) publishCmd(cmd *commands.PublishAPICommand) (response.Response, error) {
	channel := cmd.Channel
	data := cmd.Data
	err := app.publish(channel, data, cmd.Client, nil, false)
	resp := response.NewAPIPublishResponse()
	if err != nil {
		resp.SetErr(response.ResponseError{err, response.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// broadcastCmd publishes data into multiple channels.
func (app *Application) broadcastCmd(cmd *commands.BroadcastAPICommand) (response.Response, error) {
	resp := response.NewAPIBroadcastResponse()
	channels := cmd.Channels
	data := cmd.Data
	if len(channels) == 0 {
		logger.ERROR.Println("channels required for broadcast")
		resp.SetErr(response.ResponseError{ErrInvalidMessage, response.ErrorAdviceNone})
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
		resp.SetErr(response.ResponseError{firstErr, response.ErrorAdviceNone})
	}
	return resp, nil
}

// unsubscribeCmd unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes.
func (app *Application) unsubcribeCmd(cmd *commands.UnsubscribeAPICommand) (response.Response, error) {
	resp := response.NewAPIUnsubscribeResponse()
	channel := cmd.Channel
	user := cmd.User
	err := app.Unsubscribe(user, channel)
	if err != nil {
		resp.SetErr(response.ResponseError{err, response.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// disconnectCmd disconnects user by its ID and sends disconnect
// control message to other nodes so they could also disconnect this user.
func (app *Application) disconnectCmd(cmd *commands.DisconnectAPICommand) (response.Response, error) {
	resp := response.NewAPIDisconnectResponse()
	user := cmd.User
	err := app.Disconnect(user)
	if err != nil {
		resp.SetErr(response.ResponseError{err, response.ErrorAdviceNone})
		return resp, nil
	}
	return resp, nil
}

// presenceCmd returns response with presense information for channel.
func (app *Application) presenceCmd(cmd *commands.PresenceAPICommand) (response.Response, error) {
	channel := cmd.Channel
	body := response.PresenceBody{
		Channel: channel,
	}
	presence, err := app.Presence(channel)
	if err != nil {
		resp := response.NewAPIPresenceResponse(body)
		resp.SetErr(response.ResponseError{err, response.ErrorAdviceNone})
		return resp, nil
	}
	body.Data = presence
	return response.NewAPIPresenceResponse(body), nil
}

// historyCmd returns response with history information for channel.
func (app *Application) historyCmd(cmd *commands.HistoryAPICommand) (response.Response, error) {
	channel := cmd.Channel
	body := response.HistoryBody{
		Channel: channel,
	}
	history, err := app.History(channel)
	if err != nil {
		resp := response.NewAPIHistoryResponse(body)
		resp.SetErr(response.ResponseError{err, response.ErrorAdviceNone})
		return resp, nil
	}
	body.Data = history
	return response.NewAPIHistoryResponse(body), nil
}

// channelsCmd returns active channels.
func (app *Application) channelsCmd() (response.Response, error) {
	body := response.ChannelsBody{}
	channels, err := app.channels()
	if err != nil {
		logger.ERROR.Println(err)
		resp := response.NewAPIChannelsResponse(body)
		resp.SetErr(response.ResponseError{ErrInternalServerError, response.ErrorAdviceNone})
		return resp, nil
	}
	body.Data = channels
	return response.NewAPIChannelsResponse(body), nil
}

// statsCmd returns active node stats.
func (app *Application) statsCmd() (response.Response, error) {
	body := response.StatsBody{}
	body.Data = app.stats()
	return response.NewAPIStatsResponse(body), nil
}

// nodeCmd returns simple counter metrics which update in real time for the current node only.
func (app *Application) nodeCmd() (response.Response, error) {
	body := response.NodeBody{}
	body.Data = app.node()
	return response.NewAPINodeResponse(body), nil
}
