package main

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"

	"github.com/mitchellh/mapstructure"
	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
	"gopkg.in/centrifugal/sockjs-go.v2/sockjs"
)

type connection interface {
	GetUid() string
	GetProject() string
	GetUser() string
}

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

func (c *client) GetUid() string {
	return c.uid
}

func (c *client) GetProject() string {
	return c.project
}

func (c *client) GetUser() string {
	return c.user
}

func (c *client) getInfo() map[string]interface{} {

	// TODO: implement this

	return map[string]interface{}{}
}

type Params map[string]interface{}

type clientCommand struct {
	Method string
	Params Params
}

type clientCommands []clientCommand

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

func (c *client) handleCommand(command clientCommand) (response, error) {
	var err error
	var resp response
	method := command.Method
	params := command.Params

	if method != "connect" && !c.isAuthenticated {
		return response{}, ErrUnauthorized
	}

	switch method {
	case "connect":
		resp, err = c.handleConnect(params)
	case "subscribe":
		resp, err = c.handleSubscribe(params)
	case "publish":
		resp, err = c.handlePublish(params)
	default:
		return response{}, ErrMethodNotFound
	}
	if err != nil {
		return response{}, err
	}

	resp.Method = method
	return resp, nil
}

type connectCommand struct {
	Project   string
	User      string
	Timestamp string
	Info      string
	Token     string
}

// handleConnect handles connect command from client - client must send this
// command immediately after establishing Websocket or SockJS connection with
// Centrifuge
func (c *client) handleConnect(ps Params) (response, error) {

	resp := response{
		Body:   nil,
		Error:  nil,
		Method: "connect",
	}

	if c.isAuthenticated {
		resp.Body = c.uid
		return resp, nil
	}

	var cmd connectCommand
	err := mapstructure.Decode(ps, &cmd)
	if err != nil {
		return resp, ErrInvalidClientMessage
	}

	projectKey := cmd.Project
	user := cmd.User
	info := cmd.Info
	if info == "" {
		info = "{}"
	}
	timestamp := cmd.Timestamp
	token := cmd.Token

	project, exists := c.app.structure.getProjectByKey(projectKey)
	if !exists {
		return resp, ErrProjectNotFound
	}

	isValid := checkClientToken(project.Secret, projectKey, user, timestamp, info, token)
	if !isValid {
		log.Println("invalid token for user", user)
		return resp, ErrInvalidToken
	}

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		log.Println(err)
		return resp, ErrInvalidClientMessage
	}

	c.timestamp = ts
	c.user = user
	c.project = projectKey

	var defaultInfo interface{}
	err = json.Unmarshal([]byte(info), &defaultInfo)
	if err != nil {
		log.Println(err)
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

type subscribeCommand struct {
	Channel string
	Client  string
	Info    string
	Sign    string
}

// handleSubscribe handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel
func (c *client) handleSubscribe(ps Params) (response, error) {
	resp := response{
		Body:   nil,
		Error:  nil,
		Method: "subscribe",
	}

	var cmd subscribeCommand
	err := mapstructure.Decode(ps, &cmd)
	if err != nil {
		return resp, ErrInvalidClientMessage
	}

	project, exists := c.app.structure.getProjectByKey(c.project)
	if !exists {
		return resp, ErrProjectNotFound
	}
	log.Println(project)

	channel := cmd.Channel
	if channel == "" {
		return resp, ErrInvalidClientMessage
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

	channelOptions := c.app.structure.getChannelOptions(c.project, channel)
	log.Println(channelOptions)

	// TODO: check anonymous access

	if isPrivateChannel(channel) {
		// TODO: check provided sign
	}

	// TODO: add subscription using engine

	c.channels[channel] = true

	info := c.getInfo()
	log.Println(info)

	// TODO: add presence info for this channel

	// TODO: send join/leave message

	return resp, nil
}

type publishCommand struct {
	Channel string
	Data    interface{}
}

// handlePublish handles publish command - clients can publish messages into channels
// themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) handlePublish(ps Params) (response, error) {

	resp := response{
		Body:   nil,
		Error:  nil,
		Method: "publish",
	}

	var cmd publishCommand
	err := mapstructure.Decode(ps, &cmd)
	if err != nil {
		return resp, ErrInvalidClientMessage
	}

	project, exists := c.app.structure.getProjectByKey(c.project)
	if !exists {
		return resp, ErrProjectNotFound
	}
	log.Println(project)

	channel := cmd.Channel

	body := map[string]interface{}{
		"channel": channel,
		"status":  false,
	}

	// TODO: check that client subscribed on this channel

	channelOptions := c.app.structure.getChannelOptions(c.project, channel)
	log.Println(channelOptions)

	// TODO: check that publishing allowed

	info := c.getInfo()

	status, err := c.app.processPublish(project, channel, cmd.Data, info)
	body["status"] = status
	resp.Body = body

	if err != nil {
		resp.Error = err
	}

	return resp, nil
}

// printIsAuthenticated prints if client authenticated - this is just for debugging
// during development
func (c *client) printIsAuthenticated() {
	log.Println(c.isAuthenticated)
}
