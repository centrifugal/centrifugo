package main

import (
	"github.com/centrifugal/centrifugo/logger"
	"github.com/mitchellh/mapstructure"
)

// handleApiCommand builds API command and dispatches it into correct handler method
func (app *application) handleApiCommand(p *project, command apiCommand) (*response, error) {

	var err error
	var resp *response

	method := command.Method
	params := command.Params

	switch method {
	case "publish":
		var cmd publishApiCommand
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.handlePublishCommand(p, &cmd)
	case "unsubscribe":
		var cmd unsubscribeApiCommand
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.handleUnsubscribeCommand(p, &cmd)
	case "disconnect":
		var cmd disconnectApiCommand
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.handleDisconnectCommand(p, &cmd)
	case "presence":
		var cmd presenceApiCommand
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.handlePresenceCommand(p, &cmd)
	case "history":
		var cmd historyApiCommand
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidApiMessage
		}
		resp, err = app.handleHistoryCommand(p, &cmd)
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// handlePublishCommand publishes data into channel
func (app *application) handlePublishCommand(p *project, cmd *publishApiCommand) (*response, error) {

	resp := newResponse("publish")

	channel := cmd.Channel
	data := cmd.Data
	if channel == "" || data == "" {
		logger.ERROR.Println("channel and data required")
		return nil, ErrInvalidApiMessage
	}

	channelOptions := app.getChannelOptions(p.Name, channel)
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	err := app.publishClientMessage(p, channel, data, nil)
	if err != nil {
		resp.Error = ErrInternalServerError
	}

	return resp, nil
}

// handleUnsubscribeCommand unsubscribes project's user from channel and sends
// unsubscribe control message to other nodes
func (app *application) handleUnsubscribeCommand(p *project, cmd *unsubscribeApiCommand) (*response, error) {

	resp := newResponse("unsubscribe")

	channel := cmd.Channel
	user := cmd.User

	if user == "" {
		logger.ERROR.Println("user required")
		return nil, ErrInvalidApiMessage
	}

	if channel != "" {
		channelOptions := app.getChannelOptions(p.Name, channel)
		if channelOptions == nil {
			resp.Error = ErrNamespaceNotFound
			return resp, nil
		}
	}

	err := app.unsubscribeUserFromChannel(p.Name, user, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	err = app.publishUnsubscribeControlMessage(p.Name, user, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	return resp, nil
}

// handleDisconnectCommand disconnects project's user and sends disconnect
// control message to other nodes
func (app *application) handleDisconnectCommand(p *project, cmd *disconnectApiCommand) (*response, error) {

	resp := newResponse("disconnect")

	user := cmd.User

	if user == "" {
		logger.ERROR.Println("user required")
		return nil, ErrInvalidApiMessage
	}

	err := app.disconnectUser(p.Name, user)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	err = app.publishDisconnectControlMessage(p.Name, user)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	return resp, nil
}

// handlePresenceCommand returns response with presense information for project channel
func (app *application) handlePresenceCommand(p *project, cmd *presenceApiCommand) (*response, error) {

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

	channelOptions := app.getChannelOptions(p.Name, channel)
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	if !channelOptions.Presence {
		resp.Error = ErrNotAvailable
		return resp, nil
	}

	presence, err := app.getPresence(p.Name, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    presence,
	}
	return resp, nil
}

// handleHistoryCommand returns response with history information for project channel
func (app *application) handleHistoryCommand(p *project, cmd *historyApiCommand) (*response, error) {

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

	channelOptions := app.getChannelOptions(p.Name, channel)
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	if channelOptions.HistorySize <= 0 {
		resp.Error = ErrNotAvailable
		return resp, nil
	}

	history, err := app.getHistory(p.Name, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    history,
	}
	return resp, nil
}
