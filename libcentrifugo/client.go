package libcentrifugo

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/nu7hatch/gouuid"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/stringqueue"
)

const (
	CloseStatus = 3000
)

// client represents clien connection to Centrifugo - at moment this can be Websocket
// or SockJS connection. It abstracts away protocol of incoming connection having
// session interface. Session allows to Send messages via connection and to Close connection.
type client struct {
	sync.RWMutex
	app           *Application
	sess          session
	UID           ConnID
	Project       ProjectKey
	User          UserID
	timestamp     int64
	defaultInfo   []byte
	authenticated bool
	channelInfo   map[Channel][]byte
	Channels      map[Channel]bool
	messages      stringqueue.StringQueue
	closeChan     chan struct{}
	expireTimer   *time.Timer
}

// ClientInfo contains information about client to use in message
// meta information, presence information, join/leave events etc.
type ClientInfo struct {
	User        UserID           `json:"user"`
	Client      ConnID           `json:"client"`
	DefaultInfo *json.RawMessage `json:"default_info"`
	ChannelInfo *json.RawMessage `json:"channel_info"`
}

// newClient creates new ready to communicate client.
func newClient(app *Application, s session) (*client, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	c := client{
		UID:       ConnID(uid.String()),
		app:       app,
		sess:      s,
		messages:  stringqueue.New(),
		closeChan: make(chan struct{}),
	}
	go c.sendMessages()
	return &c, nil
}

// sendMessages waits for messages from queue and sends them to client.
func (c *client) sendMessages() {
	for {
		msg, ok := c.messages.Wait()
		if !ok {
			if c.messages.Closed() {
				return
			}
			continue
		}
		err := c.sendMsgTimeout(msg)
		if err != nil {
			logger.INFO.Println("error sending to", c.uid(), err.Error())
			c.sess.Close(CloseStatus, "error sending message")
		}
	}
}

func (c *client) sendMsgTimeout(msg string) error {
	c.app.RLock()
	sendTimeout := c.app.config.MessageSendTimeout
	c.app.RUnlock()
	if sendTimeout > 0 {
		// Send to client's session with provided timeout.
		to := time.After(time.Second * time.Duration(sendTimeout))
		sent := make(chan error)
		go func() {
			sent <- c.sess.Send(msg)
		}()
		select {
		case err := <-sent:
			return err
		case <-to:
			return ErrSendTimeout
		}
	} else {
		// Do not use any timeout when sending, it's recommended to keep
		// Centrifugo behind proper configured reversed proxy.
		return c.sess.Send(msg)
	}
	panic("unreachable")
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect
func (c *client) updateChannelPresence(ch Channel) {
	chOpts, err := c.app.channelOpts(c.Project, ch)
	if err != nil {
		return
	}
	if !chOpts.Presence {
		return
	}
	c.app.addPresence(c.Project, ch, c.UID, c.info(ch))
}

// updatePresence updates presence info for all client channels
func (c *client) updatePresence() {
	c.RLock()
	defer c.RUnlock()
	for _, channel := range c.channels() {
		c.updateChannelPresence(channel)
	}
}

// presencePing periodically updates presence info
func (c *client) presencePing() {
	for {
		c.app.RLock()
		interval := c.app.config.PresencePingInterval
		c.app.RUnlock()
		select {
		case <-c.closeChan:
			return
		case <-time.After(time.Duration(interval) * time.Second):
		}
		c.updatePresence()
	}
}

func (c *client) uid() ConnID {
	return c.UID
}

func (c *client) project() ProjectKey {
	return c.Project
}

func (c *client) user() UserID {
	return c.User
}

func (c *client) channels() []Channel {
	c.RLock()
	defer c.RUnlock()
	keys := make([]Channel, len(c.Channels))
	i := 0
	for k := range c.Channels {
		keys[i] = k
		i += 1
	}
	return keys
}

func (c *client) unsubscribe(ch Channel) error {
	c.Lock()
	defer c.Unlock()
	cmd := &unsubscribeClientCommand{
		Channel: ch,
	}
	resp, err := c.unsubscribeCmd(cmd)
	if err != nil {
		return err
	}
	if resp.err != nil {
		return resp.err
	}
	return nil
}

func (c *client) send(message string) error {
	ok := c.messages.Add(message)
	if !ok {
		return ErrClientClosed
	}
	return nil
}

func (c *client) close(reason string) error {
	// TODO: better locking for client - at moment we close message queue in 2 places, here and in clean() method
	c.messages.Close()
	return c.sess.Close(CloseStatus, reason)
}

// clean called when connection was closed to make different clean up
// actions for a client
func (c *client) clean() error {
	c.Lock()
	defer c.Unlock()

	pk := c.Project

	if pk != "" && len(c.Channels) > 0 {
		// unsubscribe from all channels
		for channel, _ := range c.Channels {
			cmd := &unsubscribeClientCommand{
				Channel: channel,
			}
			_, err := c.unsubscribeCmd(cmd)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	if pk != "" {
		err := c.app.removeConn(c)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	close(c.closeChan)
	c.messages.Close()

	if pk != "" && c.app.mediator != nil {
		c.app.mediator.Disconnect(pk, c.UID, c.User)
	}

	return nil
}

func (c *client) info(ch Channel) ClientInfo {
	channelInfo, ok := c.channelInfo[ch]
	if !ok {
		channelInfo = []byte{}
	}
	var rawDefaultInfo *json.RawMessage
	var rawChannelInfo *json.RawMessage
	if len(c.defaultInfo) > 0 {
		raw := json.RawMessage(c.defaultInfo)
		rawDefaultInfo = &raw
	} else {
		rawDefaultInfo = nil
	}
	if len(channelInfo) > 0 {
		raw := json.RawMessage(channelInfo)
		rawChannelInfo = &raw
	} else {
		rawChannelInfo = nil
	}
	return ClientInfo{
		User:        c.User,
		Client:      c.UID,
		DefaultInfo: rawDefaultInfo,
		ChannelInfo: rawChannelInfo,
	}
}

func cmdFromClientMsg(msgBytes []byte) ([]clientCommand, error) {
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

func (c *client) message(msg []byte) error {
	if len(msg) == 0 {
		logger.ERROR.Println("empty client message received")
		return ErrInvalidMessage
	}
	commands, err := cmdFromClientMsg(msg)
	if err != nil {
		return err
	}
	if len(commands) == 0 {
		logger.ERROR.Println("no commands in message")
		return ErrInvalidMessage
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
		resp, err := c.handleCmd(command)
		if err != nil {
			return err
		}
		resp.UID = command.UID
		mr = append(mr, resp)
	}
	jsonResp, err := json.Marshal(mr)
	if err != nil {
		return err
	}
	err = c.send(string(jsonResp))
	return err
}

// handleCmd dispatches clientCommand into correct command handler
func (c *client) handleCmd(command clientCommand) (*response, error) {

	var err error
	var resp *response

	method := command.Method
	params := command.Params

	if method != "connect" && !c.authenticated {
		return nil, ErrUnauthorized
	}

	switch method {
	case "connect":
		var cmd connectClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.connectCmd(&cmd)
	case "refresh":
		var cmd refreshClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.refreshCmd(&cmd)
	case "subscribe":
		var cmd subscribeClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.subscribeCmd(&cmd)
	case "unsubscribe":
		var cmd unsubscribeClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.unsubscribeCmd(&cmd)
	case "publish":
		var cmd publishClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.publishCmd(&cmd)
	case "ping":
		resp, err = c.pingCmd()
	case "presence":
		var cmd presenceClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.presenceCmd(&cmd)
	case "history":
		var cmd historyClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.historyCmd(&cmd)
	default:
		return nil, ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// pingCmd handles ping command from client - this is necessary sometimes
// for example Heroku closes websocket connection after 55 seconds
// of inactive period when no messages with payload travelled over wire
func (c *client) pingCmd() (*response, error) {
	resp := newResponse("ping")
	resp.Body = "pong"
	return resp, nil
}

func (c *client) expire() {
	c.Lock()
	defer c.Unlock()

	project, exists := c.app.projectByKey(c.Project)
	if !exists {
		return
	}

	if project.ConnLifetime <= 0 {
		return
	}

	timeToExpire := c.timestamp + project.ConnLifetime - time.Now().Unix()
	if timeToExpire > 0 {
		// connection was succesfully refreshed
		return
	}

	c.close("expired")
	return
}

// connectCmd handles connect command from client - client must send this
// command immediately after establishing Websocket or SockJS connection with
// Centrifugo
func (c *client) connectCmd(cmd *connectClientCommand) (*response, error) {

	resp := newResponse("connect")

	if c.authenticated {
		logger.ERROR.Println("wrong connect message: client already authenticated")
		return nil, ErrInvalidMessage
	}

	pk := cmd.Project
	user := cmd.User
	info := cmd.Info

	c.app.RLock()
	insecure := c.app.config.Insecure
	closeDelay := c.app.config.ExpiredConnectionCloseDelay
	c.app.RUnlock()

	var timestamp string
	var token string
	if !insecure {
		timestamp = cmd.Timestamp
		token = cmd.Token
	} else {
		timestamp = ""
		token = ""
	}

	project, exists := c.app.projectByKey(pk)
	if !exists {
		return nil, ErrProjectNotFound
	}

	if !insecure {
		isValid := auth.CheckClientToken(project.Secret, string(pk), string(user), timestamp, info, token)
		if !isValid {
			logger.ERROR.Println("invalid token for user", user)
			return nil, ErrInvalidToken
		}
	}

	if !insecure {
		ts, err := strconv.Atoi(timestamp)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInvalidMessage
		}
		c.timestamp = int64(ts)
	} else {
		c.timestamp = time.Now().Unix()
	}

	c.User = user
	c.Project = pk

	body := &connectBody{}

	var timeToExpire int64 = 0

	connLifetime := project.ConnLifetime
	if connLifetime > 0 && !insecure {
		timeToExpire := c.timestamp + connLifetime - time.Now().Unix()
		if timeToExpire <= 0 {
			body.Expired = true
			body.TTL = &connLifetime
			resp.Body = body
			return resp, nil
		}
	}

	c.authenticated = true
	c.defaultInfo = []byte(info)
	c.Channels = map[Channel]bool{}
	c.channelInfo = map[Channel][]byte{}

	go c.presencePing()

	err := c.app.addConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	if c.app.mediator != nil {
		c.app.mediator.Connect(c.Project, c.UID, c.User)
	}

	if timeToExpire > 0 {
		duration := time.Duration(timeToExpire+closeDelay) * time.Second
		c.expireTimer = time.AfterFunc(duration, c.expire)
	}

	body.Client = &c.UID
	if connLifetime > 0 {
		body.TTL = &connLifetime
	}
	resp.Body = body
	return resp, nil
}

// refreshCmd handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime project option set.
func (c *client) refreshCmd(cmd *refreshClientCommand) (*response, error) {

	resp := newResponse("refresh")

	pk := cmd.Project
	user := cmd.User
	info := cmd.Info
	timestamp := cmd.Timestamp
	token := cmd.Token

	project, exists := c.app.projectByKey(pk)
	if !exists {
		return nil, ErrProjectNotFound
	}

	isValid := auth.CheckClientToken(project.Secret, string(pk), string(user), timestamp, info, token)
	if !isValid {
		logger.ERROR.Println("invalid refresh token for user", user)
		return nil, ErrInvalidToken
	}

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInvalidMessage
	}

	body := &refreshBody{}

	connLifetime := project.ConnLifetime
	if connLifetime > 0 {
		// connection check enabled
		timeToExpire := int64(ts) + connLifetime - time.Now().Unix()
		if timeToExpire > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.timestamp = int64(ts)
			c.defaultInfo = []byte(info)
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(timeToExpire+c.app.config.ExpiredConnectionCloseDelay) * time.Second
			c.expireTimer = time.AfterFunc(duration, c.expire)
		} else {
			return nil, ErrConnectionExpired
		}
		body.TTL = &connLifetime
	}
	resp.Body = body
	return resp, nil
}

// subscribeCmd handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel
func (c *client) subscribeCmd(cmd *subscribeClientCommand) (*response, error) {

	resp := newResponse("subscribe")

	project, exists := c.app.projectByKey(c.Project)
	if !exists {
		return nil, ErrProjectNotFound
	}

	channel := cmd.Channel
	if channel == "" {
		return nil, ErrInvalidMessage
	}

	c.app.RLock()
	maxChannelLength := c.app.config.MaxChannelLength
	insecure := c.app.config.Insecure
	c.app.RUnlock()

	if len(channel) > maxChannelLength {
		resp.Err(ErrLimitExceeded)
		return resp, nil
	}

	body := &subscribeBody{
		Channel: channel,
	}
	resp.Body = body

	if !c.app.userAllowed(channel, c.User) || !c.app.clientAllowed(channel, c.UID) {
		resp.Err(ErrPermissionDenied)
		return resp, nil
	}

	chOpts, err := c.app.channelOpts(c.Project, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	if !chOpts.Anonymous && c.User == "" && !insecure {
		resp.Err(ErrPermissionDenied)
		return resp, nil
	}

	if c.app.privateChannel(channel) {
		// private channel - subscription must be properly signed
		if string(c.UID) != string(cmd.Client) {
			resp.Err(ErrPermissionDenied)
			return resp, nil
		}
		isValid := auth.CheckChannelSign(project.Secret, string(cmd.Client), string(channel), cmd.Info, cmd.Sign)
		if !isValid {
			resp.Err(ErrPermissionDenied)
			return resp, nil
		}
		c.channelInfo[channel] = []byte(cmd.Info)
	}

	c.Channels[channel] = true

	info := c.info(channel)

	err = c.app.addSub(c.Project, channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		return resp, ErrInternalServerError
	}

	if chOpts.Presence {
		err = c.app.addPresence(c.Project, channel, c.UID, info)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInternalServerError
		}
	}

	if chOpts.JoinLeave {
		err = c.app.pubJoinLeave(c.Project, channel, "join", info)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	if c.app.mediator != nil {
		c.app.mediator.Subscribe(c.Project, channel, c.UID, c.User)
	}

	return resp, nil
}

// unsubscribeCmd handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *client) unsubscribeCmd(cmd *unsubscribeClientCommand) (*response, error) {

	resp := newResponse("unsubscribe")

	channel := cmd.Channel
	if channel == "" {
		return nil, ErrInvalidMessage
	}

	body := &unsubscribeBody{
		Channel: channel,
	}
	resp.Body = body

	chOpts, err := c.app.channelOpts(c.Project, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	info := c.info(channel)

	_, ok := c.Channels[channel]
	if ok {

		delete(c.Channels, channel)

		err = c.app.removePresence(c.Project, channel, c.UID)
		if err != nil {
			logger.ERROR.Println(err)
		}

		if chOpts.JoinLeave {
			err = c.app.pubJoinLeave(c.Project, channel, "leave", info)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	err = c.app.removeSub(c.Project, channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		return resp, ErrInternalServerError
	}

	if c.app.mediator != nil {
		c.app.mediator.Unsubscribe(c.Project, channel, c.UID, c.User)
	}

	return resp, nil
}

// publishCmd handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) publishCmd(cmd *publishClientCommand) (*response, error) {

	resp := newResponse("publish")

	channel := cmd.Channel
	data := cmd.Data

	body := &publishBody{
		Channel: channel,
		Status:  false,
	}
	resp.Body = body

	if _, ok := c.Channels[channel]; !ok {
		resp.Err(ErrPermissionDenied)
		return resp, nil
	}

	info := c.info(channel)

	err := c.app.publish(c.Project, channel, data, c.UID, &info, true)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	// message successfully sent
	body.Status = true

	return resp, nil
}

// presenceCmd handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *client) presenceCmd(cmd *presenceClientCommand) (*response, error) {

	resp := newResponse("presence")

	channel := cmd.Channel

	body := &PresenceBody{
		Channel: channel,
	}

	resp.Body = body

	if _, ok := c.Channels[channel]; !ok {
		resp.Err(ErrPermissionDenied)
		return resp, nil
	}

	presence, err := c.app.Presence(c.Project, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	body.Data = presence

	return resp, nil
}

// historyCmd handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag)
func (c *client) historyCmd(cmd *historyClientCommand) (*response, error) {

	resp := newResponse("history")

	channel := cmd.Channel

	body := &HistoryBody{
		Channel: channel,
	}

	resp.Body = body

	if _, ok := c.Channels[channel]; !ok {
		resp.Err(ErrPermissionDenied)
		return resp, nil
	}

	history, err := c.app.History(c.Project, channel)
	if err != nil {
		resp.Err(err)
		return resp, nil
	}

	body.Data = history

	return resp, nil
}
