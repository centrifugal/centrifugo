package main

import (
	"github.com/mitchellh/mapstructure"
)

func (app *application) handleApiCommand(p *project, command apiCommand) (*response, error) {

	var err error
	var resp *response

	method := command.Method
	params := command.Params

	switch method {
	case "publish":
		resp, err = app.handlePublishCommand(p, params)
	case "unsubscribe":
		resp, err = app.handleUnsubscribeCommand(p, params)
	case "disconnect":
		resp, err = app.handleDisconnectCommand(p, params)
	case "presence":
		resp, err = app.handlePresenceCommand(p, params)
	case "history":
		resp, err = app.handleHistoryCommand(p, params)
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (app *application) handlePublishCommand(p *project, ps Params) (*response, error) {

	resp := newResponse("publish")

	var cmd publishApiCommand
	err := mapstructure.Decode(ps, &cmd)
	if err != nil {
		return nil, ErrInvalidApiMessage
	}

	channel := cmd.Channel
	data := cmd.Data

	err = app.publishClientMessage(p, channel, data, nil)
	if err != nil {
		resp.Error = ErrInternalServerError
	}

	return resp, nil
}

func (app *application) handleUnsubscribeCommand(p *project, ps Params) (*response, error) {
	return newResponse("unsubscribe"), nil
}

func (app *application) handleDisconnectCommand(p *project, ps Params) (*response, error) {
	return newResponse("disconnect"), nil
}

func (app *application) handlePresenceCommand(p *project, ps Params) (*response, error) {

	resp := newResponse("presence")

	var cmd presenceApiCommand
	err := mapstructure.Decode(ps, &cmd)
	if err != nil {
		return nil, ErrInvalidApiMessage
	}

	channel := cmd.Channel

	presence, err := app.getPresence(p.Name, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	resp.Body = presence
	return resp, nil
}

func (app *application) handleHistoryCommand(p *project, ps Params) (*response, error) {

	resp := newResponse("history")

	var cmd historyApiCommand
	err := mapstructure.Decode(ps, &cmd)
	if err != nil {
		return nil, ErrInvalidApiMessage
	}

	channel := cmd.Channel

	history, err := app.getHistory(p.Name, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	resp.Body = history
	return resp, nil
}
