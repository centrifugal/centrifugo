package libcentrifugo

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// apiCmd builds API command and dispatches it into correct handler method.
func (app *Application) apiCmd(command apiCommand) (*response, error) {

	var err error
	var resp *response

	method := command.Method
	params := command.Params

	switch method {
	case "publish":
		var cmd publishApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.publishCmd(&cmd)
	case "unsubscribe":
		var cmd unsubscribeApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.unsubcribeCmd(&cmd)
	case "disconnect":
		var cmd disconnectApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.disconnectCmd(&cmd)
	case "presence":
		var cmd presenceApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.presenceCmd(&cmd)
	case "history":
		var cmd historyApiCommand
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
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	resp.UID = command.UID

	return resp, nil
}

// publishCmd publishes data into channel.
func (app *Application) publishCmd(cmd *publishApiCommand) (*response, error) {
	resp := newResponse("publish")
	channel := cmd.Channel
	data := cmd.Data
	err := app.publish(channel, data, cmd.Client, nil, false)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	return resp, nil
}

// unsubscribeCmd unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes.
func (app *Application) unsubcribeCmd(cmd *unsubscribeApiCommand) (*response, error) {
	resp := newResponse("unsubscribe")
	channel := cmd.Channel
	user := cmd.User
	err := app.Unsubscribe(user, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	return resp, nil
}

// disconnectCmd disconnects user by its ID and sends disconnect
// control message to other nodes so they could also disconnect this user.
func (app *Application) disconnectCmd(cmd *disconnectApiCommand) (*response, error) {
	resp := newResponse("disconnect")
	user := cmd.User
	err := app.Disconnect(user)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	return resp, nil
}

// presenceCmd returns response with presense information for channel.
func (app *Application) presenceCmd(cmd *presenceApiCommand) (*response, error) {
	resp := newResponse("presence")
	channel := cmd.Channel
	body := &PresenceBody{
		Channel: channel,
	}
	resp.Body = body
	presence, err := app.Presence(channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	body.Data = presence
	return resp, nil
}

// historyCmd returns response with history information for channel.
func (app *Application) historyCmd(cmd *historyApiCommand) (*response, error) {
	resp := newResponse("history")
	channel := cmd.Channel
	body := &HistoryBody{
		Channel: channel,
	}
	resp.Body = body
	history, err := app.History(channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	body.Data = history
	return resp, nil
}

// channelsCmd returns active channels.
func (app *Application) channelsCmd() (*response, error) {
	resp := newResponse("channels")
	body := &ChannelsBody{}
	resp.Body = body
	channels, err := app.channels()
	if err != nil {
		logger.ERROR.Println(err)
		resp.Err(ErrInternalServerError)
		return resp, nil
	}
	body.Data = channels
	return resp, nil
}

// statsCmd returns active node stats.
func (app *Application) statsCmd() (*response, error) {
	resp := newResponse("stats")
	body := &StatsBody{}
	body.Data = app.stats()
	resp.Body = body
	return resp, nil
}
