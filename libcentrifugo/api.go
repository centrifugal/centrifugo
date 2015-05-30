package libcentrifugo

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// apiCmd builds API command and dispatches it into correct handler method
func (app *application) apiCmd(p *project, command apiCommand) (*response, error) {

	var err error
	var resp *response

	method := command.Method
	params := command.Params

	switch method {
	case "publish":
		var cmd publishApiCommand
		err := json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.publishCmd(p, &cmd)
	case "unsubscribe":
		var cmd unsubscribeApiCommand
		err := json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.unsubcribeCmd(p, &cmd)
	case "disconnect":
		var cmd disconnectApiCommand
		err := json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.disconnectCmd(p, &cmd)
	case "presence":
		var cmd presenceApiCommand
		err := json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.presenceCmd(p, &cmd)
	case "history":
		var cmd historyApiCommand
		err := json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.historyCmd(p, &cmd)
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// publishCmd publishes data into channel
func (app *application) publishCmd(p *project, cmd *publishApiCommand) (*response, error) {

	resp := newResponse("publish")

	channel := cmd.Channel
	data := cmd.Data
	if channel == "" || len(data) == 0 {
		logger.ERROR.Println("channel and data required")
		return nil, ErrInvalidApiMessage
	}

	chOpts, err := app.channelOpts(p.Name, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	err = app.pubClient(p, channel, chOpts, data, nil)
	if err != nil {
		logger.ERROR.Println(err)
		resp.Err(ErrInternalServerError)
	}

	return resp, nil
}

// unsubscribeCmd unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes
func (app *application) unsubcribeCmd(p *project, cmd *unsubscribeApiCommand) (*response, error) {

	resp := newResponse("unsubscribe")

	channel := cmd.Channel
	user := cmd.User

	if user == "" {
		logger.ERROR.Println("user required")
		return nil, ErrInvalidApiMessage
	}

	if channel != "" {
		_, err := app.channelOpts(p.Name, channel)
		if err != nil {
			resp.Err(err)
			return resp, nil
		}
	}

	err := app.unsubUser(p.Name, user, channel)
	if err != nil {
		resp.Err(ErrInternalServerError)
		return resp, nil
	}

	err = app.pubUnsub(p.Name, user, channel)
	if err != nil {
		resp.Err(ErrInternalServerError)
		return resp, nil
	}

	return resp, nil
}

// disconnectCmd disconnects project's user and sends disconnect
// control message to other nodes
func (app *application) disconnectCmd(p *project, cmd *disconnectApiCommand) (*response, error) {

	resp := newResponse("disconnect")

	user := cmd.User

	if user == "" {
		logger.ERROR.Println("user required")
		return nil, ErrInvalidApiMessage
	}

	err := app.disconnectUser(p.Name, user)
	if err != nil {
		resp.Err(ErrInternalServerError)
		return resp, nil
	}

	err = app.pubDisconnect(p.Name, user)
	if err != nil {
		resp.Err(ErrInternalServerError)
		return resp, nil
	}

	return resp, nil
}

// presenceCmd returns response with presense information for project channel
func (app *application) presenceCmd(p *project, cmd *presenceApiCommand) (*response, error) {

	resp := newResponse("presence")

	channel := cmd.Channel

	if channel == "" {
		logger.ERROR.Println("channel required")
		return nil, ErrInvalidApiMessage
	}

	body := map[string]interface{}{
		"channel": channel,
	}

	resp.Body = body

	chOpts, err := app.channelOpts(p.Name, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	if !chOpts.Presence {
		resp.Err(ErrNotAvailable)
		return resp, nil
	}

	presence, err := app.presence(p.Name, channel)
	if err != nil {
		resp.Err(ErrInternalServerError)
		return resp, nil
	}

	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    presence,
	}
	return resp, nil
}

// historyCmd returns response with history information for project channel
func (app *application) historyCmd(p *project, cmd *historyApiCommand) (*response, error) {

	resp := newResponse("history")

	channel := cmd.Channel

	if channel == "" {
		logger.ERROR.Println("channel required")
		return nil, ErrInvalidApiMessage
	}

	body := map[string]interface{}{
		"channel": channel,
	}

	resp.Body = body

	chOpts, err := app.channelOpts(p.Name, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Err(ErrNotAvailable)
		return resp, nil
	}

	history, err := app.history(p.Name, channel)
	if err != nil {
		resp.Err(ErrInternalServerError)
		return resp, nil
	}

	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    history,
	}
	return resp, nil
}
