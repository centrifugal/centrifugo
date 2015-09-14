package libcentrifugo

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// apiCmd builds API command and dispatches it into correct handler method
func (app *Application) apiCmd(p Project, command apiCommand) (*response, error) {

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
		resp, err = app.publishCmd(p, &cmd)
	case "unsubscribe":
		var cmd unsubscribeApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.unsubcribeCmd(p, &cmd)
	case "disconnect":
		var cmd disconnectApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.disconnectCmd(p, &cmd)
	case "presence":
		var cmd presenceApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.presenceCmd(p, &cmd)
	case "history":
		var cmd historyApiCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		resp, err = app.historyCmd(p, &cmd)
	case "channels":
		resp, err = app.channelsCmd(p)
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	resp.UID = command.UID

	return resp, nil
}

// publishCmd publishes data into channel
func (app *Application) publishCmd(p Project, cmd *publishApiCommand) (*response, error) {
	resp := newResponse("publish")
	channel := cmd.Channel
	data := cmd.Data
	err := app.publish(p.Name, channel, data, cmd.Client, nil, false)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	return resp, nil
}

// unsubscribeCmd unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes
func (app *Application) unsubcribeCmd(p Project, cmd *unsubscribeApiCommand) (*response, error) {
	resp := newResponse("unsubscribe")
	channel := cmd.Channel
	user := cmd.User
	err := app.Unsubscribe(p.Name, user, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	return resp, nil
}

// disconnectCmd disconnects project's user and sends disconnect
// control message to other nodes
func (app *Application) disconnectCmd(p Project, cmd *disconnectApiCommand) (*response, error) {
	resp := newResponse("disconnect")
	user := cmd.User
	err := app.Disconnect(p.Name, user)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	return resp, nil
}

// presenceCmd returns response with presense information for project channel
func (app *Application) presenceCmd(p Project, cmd *presenceApiCommand) (*response, error) {
	resp := newResponse("presence")
	channel := cmd.Channel
	body := &PresenceBody{
		Channel: channel,
	}
	resp.Body = body
	presence, err := app.Presence(p.Name, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	body.Data = presence
	return resp, nil
}

// historyCmd returns response with history information for project channel
func (app *Application) historyCmd(p Project, cmd *historyApiCommand) (*response, error) {
	resp := newResponse("history")
	channel := cmd.Channel
	body := &HistoryBody{
		Channel: channel,
	}
	resp.Body = body
	history, err := app.History(p.Name, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}
	body.Data = history
	return resp, nil
}

// channelsCmd returns active channels for project.
func (app *Application) channelsCmd(p Project) (*response, error) {
	resp := newResponse("channels")
	body := &ChannelsBody{}
	resp.Body = body
	channels, err := app.engine.channels(p.Name)
	if err != nil {
		logger.ERROR.Println(err)
		resp.Err(ErrInternalServerError)
		return resp, nil
	}
	body.Data = channels
	return resp, nil
}
