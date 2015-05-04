package libcentrifugo

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/logger"

	"github.com/mitchellh/mapstructure"
	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
)

// client represents clien connection to Centrifuge - at moment this can be Websocket
// or SockJS connection.
type client struct {
	sync.Mutex
	app             *application
	session         session
	uid             string
	project         string
	user            string
	timestamp       int64
	token           string
	info            interface{}
	isAuthenticated bool
	channelInfo     map[string]interface{}
	channels        map[string]bool
	messageChannel  chan string
	closeChannel    chan struct{}
	expireTimer     *time.Timer
}

func newClient(app *application, s session) (*client, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &client{
		uid:            uid.String(),
		app:            app,
		session:        s,
		messageChannel: make(chan string, 256),
		closeChannel:   make(chan struct{}),
	}, nil
}

// sendMessages waits for messages from messageChannel and sends them to client
func (c *client) sendMessages() {
	for {
		select {
		case message := <-c.messageChannel:
			err := c.session.Send(message)
			if err != nil {
				c.session.Close(3000, "error sending message")
			}
		case <-c.closeChannel:
			return
		}
	}
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect
func (c *client) updateChannelPresence(channel string) {
	channelOptions := c.app.getChannelOptions(c.project, channel)
	if channelOptions == nil {
		return
	}
	if !channelOptions.Presence {
		return
	}
	c.app.addPresence(c.project, channel, c.uid, c.getInfo(channel))
}

// updatePresence updates presence info for all client channels
func (c *client) updatePresence() {
	c.Lock()
	defer c.Unlock()
	for _, channel := range c.getChannels() {
		c.updateChannelPresence(channel)
	}
}

// presencePing periodically updates presence info
func (c *client) presencePing() {
	for {
		c.app.RLock()
		interval := c.app.config.presencePingInterval
		c.app.RUnlock()
		select {
		case <-c.closeChannel:
			return
		case <-time.After(time.Duration(interval) * time.Second):
		}
		c.updatePresence()
	}
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
	c.Lock()
	defer c.Unlock()
	cmd := &unsubscribeClientCommand{
		Channel: channel,
	}
	resp, err := c.handleUnsubscribeCommand(cmd)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (c *client) send(message string) error {
	select {
	case c.messageChannel <- message:
		return nil
	default:
		c.session.Close(3000, "error sending message")
		return ErrInternalServerError
	}
}

func (c *client) close(reason string) error {
	return c.session.Close(3000, reason)
}

// clean called when connection was closed to make different clean up
// actions for a client
func (c *client) clean() error {
	c.Lock()
	defer c.Unlock()
	projectKey := c.project

	if projectKey != "" && len(c.channels) > 0 {
		// unsubscribe from all channels
		for channel, _ := range c.channels {
			cmd := &unsubscribeClientCommand{
				Channel: channel,
			}
			_, err := c.handleUnsubscribeCommand(cmd)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	if projectKey != "" {
		err := c.app.removeConnection(c)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	close(c.closeChannel)

	return nil
}

func (c *client) getInfo(channel string) map[string]interface{} {

	var channelInfo interface{}
	channelInfo, ok := c.channelInfo[channel]
	if !ok {
		channelInfo = nil
	}

	info := map[string]interface{}{
		"user":         c.user,
		"client":       c.uid,
		"default_info": c.info,
		"channel_info": channelInfo,
	}

	return info
}

func getCommandsFromClientMessage(msgBytes []byte) ([]clientCommand, error) {
	var commands []clientCommand
	firstByte := msgBytes[0]
	switch firstByte {
	case objectJsonPrefix:
		// single command request
		var command clientCommand
		err := json.Unmarshal(msgBytes, &command)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	case arrayJsonPrefix:
		// array of commands received
		err := json.Unmarshal(msgBytes, &commands)
		if err != nil {
			return nil, err
		}
	}
	return commands, nil
}

func (c *client) handleMessage(msg []byte) error {
	if len(msg) == 0 {
		logger.ERROR.Println("empty client message received")
		return ErrInvalidClientMessage
	}
	commands, err := getCommandsFromClientMessage(msg)
	if err != nil {
		return err
	}

	err = c.handleCommands(commands)
	return err
}

func (c *client) handleCommands(commands []clientCommand) error {
	c.Lock()
	defer c.Unlock()
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

// handleCommand dispatches clientCommand into correct command handler
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
		resp, err = c.handleConnectCommand(&cmd)
	case "refresh":
		var cmd refreshClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleRefreshCommand(&cmd)
	case "subscribe":
		var cmd subscribeClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleSubscribeCommand(&cmd)
	case "unsubscribe":
		var cmd unsubscribeClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleUnsubscribeCommand(&cmd)
	case "publish":
		var cmd publishClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handlePublishCommand(&cmd)
	case "ping":
		resp, err = c.handlePingCommand()
	case "presence":
		var cmd presenceClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handlePresenceCommand(&cmd)
	case "history":
		var cmd historyClientCommand
		err = mapstructure.Decode(params, &cmd)
		if err != nil {
			return nil, ErrInvalidClientMessage
		}
		resp, err = c.handleHistoryCommand(&cmd)
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// handlePingCommand handles ping command from client - this is necessary sometimes
// for example, in the past Heroku closed websocket connection after some time
// of inactive period when no messages with payload travelled over wire
// (despite of heartbeat frames existence)
func (c *client) handlePingCommand() (*response, error) {
	resp := newResponse("ping")
	resp.Body = "pong"
	return resp, nil
}

func (c *client) expire() {
	timer := time.Tick(time.Duration(c.app.config.expiredConnectionCloseDelay) * time.Second)
	select {
	case <-timer:
	case <-c.closeChannel:
		return
	}

	project, exists := c.app.getProjectByKey(c.project)
	if !exists {
		return
	}

	if project.ConnectionLifetime <= 0 {
		return
	}

	timeToExpire := c.timestamp + project.ConnectionLifetime - time.Now().Unix()
	if timeToExpire > 0 {
		// connection was succesfully refreshed
		return
	}

	c.close("expired")
	return
}

// handleConnectCommand handles connect command from client - client must send this
// command immediately after establishing Websocket or SockJS connection with
// Centrifuge
func (c *client) handleConnectCommand(cmd *connectClientCommand) (*response, error) {

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

	var timestamp string
	var token string
	if !c.app.config.insecure {
		timestamp = cmd.Timestamp
		token = cmd.Token
	} else {
		timestamp = ""
		token = ""
	}

	project, exists := c.app.getProjectByKey(projectKey)
	if !exists {
		return nil, ErrProjectNotFound
	}

	if !c.app.config.insecure {
		isValid := checkClientToken(project.Secret, projectKey, user, timestamp, info, token)
		if !isValid {
			logger.ERROR.Println("invalid token for user", user)
			return nil, ErrInvalidToken
		}
	}

	if !c.app.config.insecure {
		ts, err := strconv.Atoi(timestamp)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidClientMessage
		}
		c.timestamp = int64(ts)
	} else {
		c.timestamp = time.Now().Unix()
	}

	c.user = user
	c.project = projectKey

	var defaultInfo interface{}
	err := json.Unmarshal([]byte(info), &defaultInfo)
	if err != nil {
		logger.ERROR.Println(err)
		defaultInfo = map[string]interface{}{}
	}

	var ttl interface{}
	var timeToExpire int64 = 0
	ttl = nil
	connectionLifetime := project.ConnectionLifetime
	if connectionLifetime > 0 && !c.app.config.insecure {
		ttl = connectionLifetime
		timeToExpire := c.timestamp + connectionLifetime - time.Now().Unix()
		if timeToExpire <= 0 {
			body := map[string]interface{}{
				"client":  nil,
				"expired": true,
				"ttl":     connectionLifetime,
			}
			resp.Body = body
			return resp, nil
		}
	}

	c.isAuthenticated = true
	c.info = defaultInfo
	c.channels = map[string]bool{}
	c.channelInfo = map[string]interface{}{}

	go c.presencePing()

	err = c.app.addConnection(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	if timeToExpire > 0 {
		duration := time.Duration(timeToExpire) * time.Second
		c.expireTimer = time.AfterFunc(duration, c.expire)
	}

	body := map[string]interface{}{
		"client":  c.uid,
		"expired": false,
		"ttl":     ttl,
	}
	resp.Body = body
	return resp, nil
}

// handleRefreshCommand handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime project option set.
func (c *client) handleRefreshCommand(cmd *refreshClientCommand) (*response, error) {

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

	var ttl interface{}

	connectionLifetime := project.ConnectionLifetime
	if connectionLifetime <= 0 {
		// connection check disabled
		ttl = nil
	} else {
		timeToExpire := int64(ts) + connectionLifetime - time.Now().Unix()
		if timeToExpire > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.timestamp = int64(ts)
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(timeToExpire) * time.Second
			c.expireTimer = time.AfterFunc(duration, c.expire)
		} else {
			return nil, ErrConnectionExpired
		}
		ttl = connectionLifetime
	}

	// return connection's time to live to the client
	body := map[string]interface{}{
		"ttl": ttl,
	}
	resp.Body = body
	return resp, nil
}

// handleSubscribeCommand handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel
func (c *client) handleSubscribeCommand(cmd *subscribeClientCommand) (*response, error) {

	resp := newResponse("subscribe")

	project, exists := c.app.getProjectByKey(c.project)
	if !exists {
		return nil, ErrProjectNotFound
	}

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

	if !c.app.isUserAllowed(channel, c.user) {
		resp.Error = ErrPermissionDenied
		return resp, nil
	}

	channelOptions := c.app.getChannelOptions(c.project, channel)
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	if !channelOptions.Anonymous && c.user == "" && !c.app.config.insecure {
		resp.Error = ErrPermissionDenied
		return resp, nil
	}

	if c.app.isPrivateChannel(channel) {
		// private channel - subscription must be properly signed
		isValid := checkChannelSign(project.Secret, cmd.Client, channel, cmd.Info, cmd.Sign)
		if !isValid {
			resp.Error = ErrPermissionDenied
			return resp, nil
		}
		if cmd.Info != "" {
			var info interface{}
			err := json.Unmarshal([]byte(cmd.Info), &info)
			if err != nil {
				logger.ERROR.Panicln(err)
			} else {
				c.channelInfo[channel] = info
			}
		}

	}

	err := c.app.addSubscription(c.project, channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		return resp, ErrInternalServerError
	}

	c.channels[channel] = true

	info := c.getInfo(channel)

	if channelOptions.Presence {
		err = c.app.addPresence(c.project, channel, c.uid, info)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInternalServerError
		}
	}

	if channelOptions.JoinLeave {
		err = c.app.publishJoinLeaveMessage(c.project, channel, "join", info)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	return resp, nil
}

// handleUnsubscribeCommand handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *client) handleUnsubscribeCommand(cmd *unsubscribeClientCommand) (*response, error) {

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
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	err := c.app.removeSubscription(c.project, channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		return resp, ErrInternalServerError
	}

	_, ok := c.channels[channel]
	if ok {

		delete(c.channels, channel)

		err = c.app.removePresence(c.project, channel, c.uid)
		if err != nil {
			logger.ERROR.Println(err)
		}

		if channelOptions.JoinLeave {
			err = c.app.publishJoinLeaveMessage(c.project, channel, "leave", c.getInfo(channel))
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	return resp, nil
}

// handlePublishCommand handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) handlePublishCommand(cmd *publishClientCommand) (*response, error) {

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
	resp.Body = body

	if _, ok := c.channels[channel]; !ok {
		resp.Error = ErrPermissionDenied
		return resp, nil
	}

	channelOptions := c.app.getChannelOptions(c.project, channel)
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	if !channelOptions.Publish && !c.app.config.insecure {
		resp.Error = ErrPermissionDenied
		return resp, nil
	}

	info := c.getInfo(channel)

	err := c.app.publishClientMessage(project, channel, data, info)
	if err != nil {
		logger.ERROR.Println(err)
		resp.Error = ErrInternalServerError
	} else {
		resp.Body = map[string]interface{}{
			"channel": channel,
			"status":  true,
		}
	}

	return resp, nil
}

// handlePresenceCommand handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *client) handlePresenceCommand(cmd *presenceClientCommand) (*response, error) {

	resp := newResponse("presence")

	channel := cmd.Channel

	if channel == "" {
		logger.ERROR.Println("channel required")
		return nil, ErrInvalidClientMessage
	}

	body := map[string]interface{}{
		"channel": channel,
	}

	resp.Body = body

	channelOptions := c.app.getChannelOptions(c.project, channel)
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	if !channelOptions.Presence {
		resp.Error = ErrNotAvailable
		return resp, nil
	}

	data, err := c.app.getPresence(c.project, channel)
	if err != nil {
		logger.ERROR.Println(err)
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    data,
	}
	return resp, nil
}

// handleHistoryCommand handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag)
func (c *client) handleHistoryCommand(cmd *historyClientCommand) (*response, error) {

	resp := newResponse("history")

	channel := cmd.Channel

	if channel == "" {
		logger.ERROR.Println("channel required")
		return nil, ErrInvalidClientMessage
	}

	body := map[string]interface{}{
		"channel": channel,
	}

	resp.Body = body

	channelOptions := c.app.getChannelOptions(c.project, channel)
	if channelOptions == nil {
		resp.Error = ErrNamespaceNotFound
		return resp, nil
	}

	if channelOptions.HistorySize <= 0 || channelOptions.HistoryLifetime <= 0 {
		resp.Error = ErrNotAvailable
		return resp, nil
	}

	data, err := c.app.getHistory(c.project, channel)
	if err != nil {
		resp.Error = ErrInternalServerError
		return resp, nil
	}

	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    data,
	}
	return resp, nil
}
