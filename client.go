package main

import (
	"encoding/json"
	"strconv"
	"sync"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/mitchellh/mapstructure"
	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
	"gopkg.in/centrifugal/sockjs-go.v2/sockjs"
)

type client struct {
	sync.Mutex
	app             *application
	session         sockjs.Session
	uid             string
	project         string
	user            string
	timestamp       int
	token           string
	info            interface{}
	channelInfo     map[string]interface{}
	isAuthenticated bool
	channels        map[string]bool
	closeChannel    chan struct{}
}

func newClient(app *application, session sockjs.Session, closeChannel chan struct{}) (*client, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &client{
		uid:          uid.String(),
		app:          app,
		session:      session,
		closeChannel: closeChannel,
	}, nil
}

func (c *client) getUid() string {
	return c.uid
}

func (c *client) getProject() string {
	return c.project
}

func (c *client) getUser() string {
	return c.user
}

func (c *client) getChannels() []string {
	keys := make([]string, len(c.channels))
	i := 0
	for k := range c.channels {
		keys[i] = k
		i += 1
	}
	return keys
}

func (c *client) unsubscribe(channel string) error {
	cmd := &unsubscribeClientCommand{
		Channel: channel,
	}
	resp, err := c.handleUnsubscribe(cmd)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (c *client) send(message string) error {
	err := c.session.Send(message)
	if err != nil {
		c.session.Close(3000, "error sending message")
	}
	return err
}

func (c *client) close(reason string) error {
	return c.session.Close(3000, reason)
}

func (c *client) clean() error {

	// TODO: stop presence ping

	projectKey := c.project

	if projectKey != "" {
		// TODO: remove from connectionHub
		logger.ERROR.Println("remove from connectionHub must be implemented")
	}

	if projectKey != "" && len(c.channels) > 0 {
		for channel, _ := range c.channels {

			// TODO: remove presence in channel

			err := c.app.removeSubscription(projectKey, channel, c)
			if err != nil {
				logger.ERROR.Println(err)
			}

			channelOptions := c.app.getChannelOptions(projectKey, channel)
			if channelOptions.JoinLeave {
				// TODO: send leave message in channel
				logger.ERROR.Println("sending leave message must be implemented")
			}
		}
	}

	// TODO: check that client and sockjs session garbage collected
	return nil
}

func (c *client) getInfo() map[string]interface{} {

	info := map[string]interface{}{
		"user":         c.user,
		"client":       c.uid,
		"default_info": c.info,
		"channel_info": map[string]interface{}{}, // TODO: implement channel_info
	}

	return info
}

func getMessageType(msgBytes []byte) (string, error) {
	var f interface{}
	err := json.Unmarshal(msgBytes, &f)
	if err != nil {
		return "", err
	}
	switch f.(type) {
	case map[string]interface{}:
		return "map", nil
	case []interface{}:
		return "array", nil
	default:
		return "", ErrInvalidClientMessage
	}
}

func getCommandsFromClientMessage(msgBytes []byte, msgType string) ([]clientCommand, error) {
	var commands []clientCommand
	switch msgType {
	case "map":
		// single command request
		var command clientCommand
		err := json.Unmarshal(msgBytes, &command)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	case "array":
		// array of commands received
		err := json.Unmarshal(msgBytes, &commands)
		if err != nil {
			return nil, err
		}
	}
	return commands, nil
}

func (c *client) handleMessage(msg string) error {
	msgBytes := []byte(msg)
	msgType, err := getMessageType(msgBytes)
	if err != nil {
		return err
	}

	commands, err := getCommandsFromClientMessage(msgBytes, msgType)
	if err != nil {
		return err
	}

	err = c.handleCommands(commands)
	return err
}

func (c *client) handleCommands(commands []clientCommand) error {
	var err error
	var mr multiResponse
	for _, command := range commands {
		resp, err := c.handleCommand(command)
		if err != nil {
			return err
		}
		mr = append(mr, resp)
	}
	jsonResp, err := mr.toJson()
	if err != nil {
		return err
	}
	err = c.session.Send(string(jsonResp))
	return err
}

func (c *client) handleCommand(command clientCommand) (*response, error) {

	var err error
	var resp *response

	method := command.Method
	params := command.Params

	if method != "connect" && !c.isAuthenticated {
		return nil, ErrUnauthorized
	}

	switch method {
	case "connect":
		var cmd connectClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleConnect(&cmd)
	case "refresh":
		var cmd refreshClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleRefresh(&cmd)
	case "subscribe":
		var cmd subscribeClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleSubscribe(&cmd)
	case "unsubscribe":
		var cmd unsubscribeClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleUnsubscribe(&cmd)
	case "publish":
		var cmd publishClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handlePublish(&cmd)
	case "ping":
		resp, err = c.handlePing()
	case "presence":
		var cmd presenceClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handlePresence(&cmd)
	case "history":
		var cmd historyClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleHistory(&cmd)
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// handlePing handles ping command from client - this is necessary sometimes
// for example, in the past Heroku closed websocket connection after some time
// of inactive period when no messages with payload travelled over wire
// (despite of heartbeat frames existence)
func (c *client) handlePing() (*response, error) {
	resp := newResponse("ping")
	resp.Body = "pong"
	return resp, nil
}

// handleConnect handles connect command from client - client must send this
// command immediately after establishing Websocket or SockJS connection with
// Centrifuge
func (c *client) handleConnect(cmd *connectClientCommand) (*response, error) {

	resp := newResponse("connect")

	if c.isAuthenticated {
		resp.Body = c.uid
		return resp, nil
	}

	projectKey := cmd.Project
	user := cmd.User
	info := cmd.Info
	if info == "" {
		info = "{}"
	}
	timestamp := cmd.Timestamp
	token := cmd.Token

	project, exists := c.app.getProjectByKey(projectKey)
	if !exists {
		return nil, ErrProjectNotFound
	}

	isValid := checkClientToken(project.Secret, projectKey, user, timestamp, info, token)
	if !isValid {
		logger.ERROR.Println("invalid token for user", user)
		return nil, ErrInvalidToken
	}

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInvalidClientMessage
	}

	c.timestamp = ts
	c.user = user
	c.project = projectKey

	var defaultInfo interface{}
	err = json.Unmarshal([]byte(info), &defaultInfo)
	if err != nil {
		logger.ERROR.Println(err)
		defaultInfo = map[string]interface{}{}
	}

	c.isAuthenticated = true
	c.info = defaultInfo
	c.channels = map[string]bool{}

	// TODO: initialize presence ping

	// TODO: add connection to application hub

	body := map[string]interface{}{
		"client":  c.uid,
		"expired": false,
		"ttl":     nil,
	}
	resp.Body = body
	return resp, nil
}

func (c *client) handleRefresh(cmd *refreshClientCommand) (*response, error) {

	resp := newResponse("refresh")

	projectKey := cmd.Project
	user := cmd.User
	info := cmd.Info
	if info == "" {
		info = "{}"
	}
	timestamp := cmd.Timestamp
	token := cmd.Token

	project, exists := c.app.getProjectByKey(projectKey)
	if !exists {
		return nil, ErrProjectNotFound
	}

	isValid := checkClientToken(project.Secret, projectKey, user, timestamp, info, token)
	if !isValid {
		logger.ERROR.Println("invalid refresh token for user", user)
		return nil, ErrInvalidToken
	}

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInvalidClientMessage
	}

	if 1 > 0 { // TODO: properly check new timestamp
		c.timestamp = ts
	} else {
		return nil, ErrConnectionExpired
	}

	// return connection's time to live to the client
	body := map[string]interface{}{
		"ttl": nil,
	}
	resp.Body = body
	return resp, nil
}

// handleSubscribe handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel
func (c *client) handleSubscribe(cmd *subscribeClientCommand) (*response, error) {

	resp := newResponse("subscribe")

	channel := cmd.Channel
	if channel == "" {
		return nil, ErrInvalidClientMessage
	}

	if len(channel) > viper.GetInt("max_channel_length") {
		resp.Error = ErrLimitExceeded
		return resp, nil
	}

	body := map[string]string{
		"channel": channel,
	}
	resp.Body = body

	// TODO: check allowed users

	channelOptions := c.app.getChannelOptions(c.project, channel)

	if !channelOptions.Anonymous && c.user == "" {
		resp.Error = ErrPermissionDenied
		return resp, nil
	}

	if c.app.isPrivateChannel(channel) {
		// TODO: check provided sign
	}

	err := c.app.addSubscription(c.project, channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		return resp, ErrInternalServerError
	}

	c.channels[channel] = true

	info := c.getInfo()

	err = c.app.addPresence(c.project, channel, c.uid, info)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	// TODO: send join message

	return resp, nil
}

func (c *client) handleUnsubscribe(cmd *unsubscribeClientCommand) (*response, error) {

	resp := newResponse("unsubscribe")

	channel := cmd.Channel
	if channel == "" {
		return nil, ErrInvalidClientMessage
	}

	body := map[string]string{
		"channel": channel,
	}
	resp.Body = body

	channelOptions := c.app.getChannelOptions(c.project, channel)
	logger.ERROR.Println(channelOptions)

	err := c.app.removeSubscription(c.project, channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		return resp, ErrInternalServerError
	}

	_, ok := c.channels[channel]
	if ok {
		delete(c.channels, channel)

		// TODO: remove presence using engine

		// send leave message into channel
	}

	return resp, nil
}

// handlePublish handles publish command - clients can publish messages into channels
// themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) handlePublish(cmd *publishClientCommand) (*response, error) {

	resp := newResponse("publish")

	project, exists := c.app.getProjectByKey(c.project)
	if !exists {
		return nil, ErrProjectNotFound
	}

	channel := cmd.Channel
	data := cmd.Data

	if channel == "" || data == "" {
		logger.ERROR.Println("channel and data required")
		return nil, ErrInvalidClientMessage
	}

	body := map[string]interface{}{
		"channel": channel,
		"status":  false,
	}

	if _, ok := c.channels[channel]; !ok {
		resp.Error = ErrPermissionDenied
		return resp, nil
	}

	channelOptions := c.app.getChannelOptions(c.project, channel)

	if !channelOptions.Publish {
		resp.Error = ErrPermissionDenied
		return resp, nil
	}

	info := c.getInfo()

	err := c.app.publishClientMessage(project, channel, data, info)
	if err != nil {
		logger.ERROR.Println(err)
		resp.Error = ErrInternalServerError
	} else {
		body["status"] = true
		resp.Body = body
	}

	return resp, nil
}

// handlePresence handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *client) handlePresence(cmd *presenceClientCommand) (*response, error) {

	resp := newResponse("presence")

	channel := cmd.Channel

	if channel == "" {
		logger.ERROR.Println("channel required")
		return nil, ErrInvalidClientMessage
	}

	channelOptions := c.app.getChannelOptions(c.project, channel)

	if !channelOptions.Presence {
		resp.Error = ErrNotAvailable
		return resp, nil
	}

	data, err := c.app.getPresence(c.project, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	body := map[string]interface{}{
		"data": data,
	}
	resp.Body = body
	return resp, nil
}

// handleHistory handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag)
func (c *client) handleHistory(cmd *historyClientCommand) (*response, error) {

	resp := newResponse("history")

	channel := cmd.Channel

	if channel == "" {
		logger.ERROR.Println("channel required")
		return nil, ErrInvalidClientMessage
	}

	channelOptions := c.app.getChannelOptions(c.project, channel)

	if channelOptions.HistorySize == 0 {
		resp.Error = ErrNotAvailable
		return resp, nil
	}

	data, err := c.app.getHistory(c.project, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	body := map[string]interface{}{
		"data": data,
	}
	resp.Body = body
	return resp, nil
}
