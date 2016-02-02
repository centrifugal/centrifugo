package libcentrifugo

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/satori/go.uuid"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/bytequeue"
)

const (
	// CloseStatus is status code set when closing client connections.
	CloseStatus = 3000
)

// client represents clien connection to Centrifugo - at moment this can be Websocket
// or SockJS connection. It abstracts away protocol of incoming connection having
// session interface. Session allows to Send messages via connection and to Close connection.
type client struct {
	sync.RWMutex
	app            *Application
	sess           session
	UID            ConnID
	User           UserID
	timestamp      int64
	defaultInfo    []byte
	authenticated  bool
	channelInfo    map[Channel][]byte
	Channels       map[Channel]bool
	messages       bytequeue.ByteQueue
	closeChan      chan struct{}
	staleTimer     *time.Timer
	expireTimer    *time.Timer
	presenceTimer  *time.Timer
	sendTimeout    time.Duration
	maxQueueSize   int
	maxRequestSize int
}

// ClientInfo contains information about client to use in message
// meta information, presence information, join/leave events etc.
type ClientInfo struct {
	User        UserID           `json:"user"`
	Client      ConnID           `json:"client"`
	DefaultInfo *json.RawMessage `json:"default_info,omitempty"`
	ChannelInfo *json.RawMessage `json:"channel_info,omitempty"`
}

// newClient creates new ready to communicate client.
func newClient(app *Application, s session) (*client, error) {
	c := client{
		UID:       ConnID(uuid.NewV4().String()),
		app:       app,
		sess:      s,
		closeChan: make(chan struct{}),
	}
	app.RLock()
	staleCloseDelay := app.config.StaleConnectionCloseDelay
	queueInitialCapacity := app.config.ClientQueueInitialCapacity
	c.maxQueueSize = app.config.ClientQueueMaxSize
	c.maxRequestSize = app.config.ClientRequestMaxSize
	c.sendTimeout = app.config.MessageSendTimeout
	app.RUnlock()
	c.messages = bytequeue.New(queueInitialCapacity)
	go c.sendMessages()
	if staleCloseDelay > 0 {
		c.staleTimer = time.AfterFunc(staleCloseDelay, c.closeUnauthenticated)
	}
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
			c.close("error sending message")
			return
		}
		c.app.metrics.numMsgSent.Inc(1)
		c.app.metrics.bytesClientOut.Inc(int64(len(msg)))
	}
}

func (c *client) sendMsgTimeout(msg []byte) error {
	sendTimeout := c.sendTimeout // No lock here as sendTimeout immutable while client exists.
	if sendTimeout > 0 {
		// Send to client's session with provided timeout.
		to := time.After(sendTimeout)
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
		// Centrifugo behind properly configured reverse proxy.
		return c.sess.Send(msg)
	}
	panic("unreachable")
}

// closeUnauthenticated closes connection if it's not authenticated yet.
// At moment used to close connections which have not sent valid connect command
// in a reasonable time interval after actually connected to Centrifugo.
func (c *client) closeUnauthenticated() {
	c.RLock()
	defer c.RUnlock()
	if !c.authenticated {
		c.close("stale")
	}
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect
func (c *client) updateChannelPresence(ch Channel) {
	chOpts, err := c.app.channelOpts(ch)
	if err != nil {
		return
	}
	if !chOpts.Presence {
		return
	}
	c.app.addPresence(ch, c.UID, c.info(ch))
}

// updatePresence updates presence info for all client channels
func (c *client) updatePresence() {
	c.app.RLock()
	presenceInterval := c.app.config.PresencePingInterval
	c.app.RUnlock()
	c.RLock()
	defer c.RUnlock()
	for _, channel := range c.channels() {
		c.updateChannelPresence(channel)
	}
	c.presenceTimer = time.AfterFunc(presenceInterval, c.updatePresence)
}

func (c *client) uid() ConnID {
	return c.UID
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
		i++
	}
	return keys
}

func (c *client) unsubscribe(ch Channel) error {
	c.Lock()
	defer c.Unlock()
	cmd := &UnsubscribeClientCommand{
		Channel: ch,
	}
	resp, err := c.unsubscribeCmd(cmd)
	if err != nil {
		return err
	}
	if resp.err != nil {
		return resp.err
	}
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.send(respJSON)
}

func (c *client) send(message []byte) error {
	ok := c.messages.Add(message)
	if !ok {
		return ErrClientClosed
	}
	c.app.metrics.numMsgQueued.Inc(1)
	if c.messages.Size() > c.maxQueueSize {
		c.close("slow")
		return ErrClientClosed
	}
	return nil
}

func (c *client) close(reason string) error {
	// TODO: better locking for client - at moment we close message queue in 2 places, here and in clean() method
	c.messages.Close()
	err := c.sess.Close(CloseStatus, reason)
	if err != nil {
		logger.ERROR.Println(err)
	}
	return err
}

// clean called when connection was closed to make different clean up
// actions for a client
func (c *client) clean() error {
	c.Lock()
	defer c.Unlock()

	select {
	case <-c.closeChan:
		return nil
	default:
		close(c.closeChan)
	}

	if len(c.Channels) > 0 {
		// unsubscribe from all channels
		for channel := range c.Channels {
			cmd := &UnsubscribeClientCommand{
				Channel: channel,
			}
			_, err := c.unsubscribeCmd(cmd)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	if c.authenticated {
		err := c.app.removeConn(c)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	c.messages.Close()

	if c.authenticated && c.app.mediator != nil {
		c.app.mediator.Disconnect(c.UID, c.User)
	}

	if c.expireTimer != nil {
		c.expireTimer.Stop()
	}

	if c.presenceTimer != nil {
		c.presenceTimer.Stop()
	}

	c.authenticated = false

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
	case objectJSONPrefix:
		// single command request
		var command clientCommand
		err := json.Unmarshal(msgBytes, &command)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	case arrayJSONPrefix:
		// array of commands received
		err := json.Unmarshal(msgBytes, &commands)
		if err != nil {
			return nil, err
		}
	}
	return commands, nil
}

func (c *client) message(msg []byte) error {
	c.app.metrics.numClientRequests.Inc(1)
	c.app.metrics.bytesClientIn.Inc(int64(len(msg)))
	defer c.app.metrics.timeClient.UpdateSince(time.Now())

	if len(msg) == 0 {
		logger.ERROR.Println("empty client message received")
		return ErrInvalidMessage
	}
	if len(msg) > c.maxRequestSize {
		logger.ERROR.Println("client request exceeds max request size limit")
		return ErrLimitExceeded
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
	if err != nil {
		reconnect := false
		if err == ErrInternalServerError {
			// In case of any internal server error we give client an advice to reconnect.
			// Any other error results in disconnect without reconnect.
			reconnect = true
		}
		c.disconnect(err.Error(), reconnect)
		if !reconnect {
			// Sleep for a while to give client a chance to receive disconnect
			// message and process it. Connection will be closed then.
			time.Sleep(time.Second)
		}
	}
	return err
}

func (c *client) disconnect(reason string, reconnect bool) error {
	resp := newClientResponse("disconnect")
	resp.Body = &DisconnectBody{
		Reason:    reason,
		Reconnect: reconnect,
	}
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.send(jsonResp)
}

func (c *client) handleCommands(commands []clientCommand) error {
	c.Lock()
	defer c.Unlock()
	var err error
	var mr multiClientResponse
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
	err = c.send(jsonResp)
	return err
}

// handleCmd dispatches clientCommand into correct command handler
func (c *client) handleCmd(command clientCommand) (*clientResponse, error) {

	var err error
	var resp *clientResponse

	method := command.Method
	params := command.Params

	if method != "connect" && !c.authenticated {
		return nil, ErrUnauthorized
	}

	switch method {
	case "connect":
		var cmd ConnectClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.connectCmd(&cmd)
	case "refresh":
		var cmd RefreshClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.refreshCmd(&cmd)
	case "subscribe":
		var cmd SubscribeClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.subscribeCmd(&cmd)
	case "unsubscribe":
		var cmd UnsubscribeClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.unsubscribeCmd(&cmd)
	case "publish":
		var cmd PublishClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.publishCmd(&cmd)
	case "ping":
		var cmd PingClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.pingCmd(&cmd)
	case "presence":
		var cmd PresenceClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, ErrInvalidMessage
		}
		resp, err = c.presenceCmd(&cmd)
	case "history":
		var cmd HistoryClientCommand
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
func (c *client) pingCmd(cmd *PingClientCommand) (*clientResponse, error) {
	resp := newClientResponse("ping")
	resp.Body = &PingBody{
		Data: cmd.Data,
	}
	return resp, nil
}

func (c *client) expire() {
	c.Lock()
	defer c.Unlock()

	c.app.RLock()
	connLifetime := c.app.config.ConnLifetime
	c.app.RUnlock()

	if connLifetime <= 0 {
		return
	}

	timeToExpire := c.timestamp + connLifetime - time.Now().Unix()
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
func (c *client) connectCmd(cmd *ConnectClientCommand) (*clientResponse, error) {

	resp := newClientResponse("connect")

	if c.authenticated {
		logger.ERROR.Println("connect error: client already authenticated")
		return nil, ErrInvalidMessage
	}

	user := cmd.User
	info := cmd.Info

	c.app.RLock()
	secret := c.app.config.Secret
	insecure := c.app.config.Insecure
	closeDelay := c.app.config.ExpiredConnectionCloseDelay
	connLifetime := c.app.config.ConnLifetime
	version := c.app.config.Version
	presenceInterval := c.app.config.PresencePingInterval
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

	if !insecure {
		isValid := auth.CheckClientToken(secret, string(user), timestamp, info, token)
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

	body := &ConnectBody{}
	body.Version = version
	body.Expires = connLifetime > 0
	body.TTL = connLifetime

	var timeToExpire int64

	if connLifetime > 0 && !insecure {
		timeToExpire = c.timestamp + connLifetime - time.Now().Unix()
		if timeToExpire <= 0 {
			body.Expired = true
			resp.Body = body
			return resp, nil
		}
	}

	c.authenticated = true
	c.defaultInfo = []byte(info)
	c.Channels = map[Channel]bool{}
	c.channelInfo = map[Channel][]byte{}

	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}

	c.presenceTimer = time.AfterFunc(presenceInterval, c.updatePresence)

	err := c.app.addConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInternalServerError
	}

	if c.app.mediator != nil {
		c.app.mediator.Connect(c.UID, c.User)
	}

	if timeToExpire > 0 {
		duration := closeDelay + time.Duration(timeToExpire)*time.Second
		c.expireTimer = time.AfterFunc(duration, c.expire)
	}

	body.Client = c.UID
	resp.Body = body
	return resp, nil
}

// refreshCmd handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime option set.
func (c *client) refreshCmd(cmd *RefreshClientCommand) (*clientResponse, error) {

	resp := newClientResponse("refresh")

	user := cmd.User
	info := cmd.Info
	timestamp := cmd.Timestamp
	token := cmd.Token

	c.app.RLock()
	secret := c.app.config.Secret
	c.app.RUnlock()

	isValid := auth.CheckClientToken(secret, string(user), timestamp, info, token)
	if !isValid {
		logger.ERROR.Println("invalid refresh token for user", user)
		return nil, ErrInvalidToken
	}

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, ErrInvalidMessage
	}

	c.app.RLock()
	closeDelay := c.app.config.ExpiredConnectionCloseDelay
	connLifetime := c.app.config.ConnLifetime
	version := c.app.config.Version
	c.app.RUnlock()

	body := &ConnectBody{}
	body.Version = version
	body.Expires = connLifetime > 0
	body.TTL = connLifetime
	body.Client = c.UID

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
			duration := time.Duration(timeToExpire)*time.Second + closeDelay
			c.expireTimer = time.AfterFunc(duration, c.expire)
		} else {
			body.Expired = true
		}
	}
	resp.Body = body
	return resp, nil
}

func recoverMessages(last MessageID, messages []Message) ([]Message, bool) {
	if last == MessageID("") {
		// Client wants to recover messages but it seems that there were no
		// messages in history before, so client missed all messages which
		// exist now.
		return messages, false
	}
	position := -1
	for index, msg := range messages {
		if msg.UID == last {
			position = index
			break
		}
	}
	if position > -1 {
		// Last uid provided found in history. Set recovered flag which means that
		// Centrifugo thinks missed messages fully recovered.
		return messages[0:position], true
	}
	// Last id provided not found in history messages. This means that client
	// most probably missed too many messages (maybe wrong last uid provided but
	// it's not a normal case). So we try to compensate as many as we can. But
	// recovered flag stays false so we do not give a guarantee all missed messages
	// recovered successfully.
	return messages, false
}

// subscribeCmd handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel. Optionally we can send missed messages to
// client if it provided last message id seen in channel.
func (c *client) subscribeCmd(cmd *SubscribeClientCommand) (*clientResponse, error) {

	resp := newClientResponse("subscribe")

	channel := cmd.Channel
	if channel == "" {
		return nil, ErrInvalidMessage
	}

	c.app.RLock()
	secret := c.app.config.Secret
	maxChannelLength := c.app.config.MaxChannelLength
	channelLimit := c.app.config.ClientChannelLimit
	insecure := c.app.config.Insecure
	c.app.RUnlock()

	if len(channel) > maxChannelLength {
		logger.ERROR.Printf("channel too long: max %d, got %d", maxChannelLength, len(channel))
		resp.Err(clientError{ErrLimitExceeded, errorAdviceFix})
		return resp, nil
	}

	if len(c.Channels) >= channelLimit {
		logger.ERROR.Printf("maximimum limit of channels per client reached: %d", channelLimit)
		resp.Err(clientError{ErrLimitExceeded, errorAdviceFix})
		return resp, nil
	}

	body := &SubscribeBody{
		Channel: channel,
	}
	resp.Body = body

	if _, ok := c.Channels[channel]; ok {
		resp.Err(clientError{ErrAlreadySubscribed, errorAdviceFix})
		return resp, nil
	}

	if !c.app.userAllowed(channel, c.User) || !c.app.clientAllowed(channel, c.UID) {
		resp.Err(clientError{ErrPermissionDenied, errorAdviceFix})
		return resp, nil
	}

	chOpts, err := c.app.channelOpts(channel)
	if err != nil {
		resp.Err(clientError{err, errorAdviceFix})
		return resp, nil
	}

	if !chOpts.Anonymous && c.User == "" && !insecure {
		resp.Err(clientError{ErrPermissionDenied, errorAdviceFix})
		return resp, nil
	}

	if c.app.privateChannel(channel) {
		// private channel - subscription must be properly signed
		if string(c.UID) != string(cmd.Client) {
			resp.Err(clientError{ErrPermissionDenied, errorAdviceFix})
			return resp, nil
		}
		isValid := auth.CheckChannelSign(secret, string(cmd.Client), string(channel), cmd.Info, cmd.Sign)
		if !isValid {
			resp.Err(clientError{ErrPermissionDenied, errorAdviceFix})
			return resp, nil
		}
		c.channelInfo[channel] = []byte(cmd.Info)
	}

	c.Channels[channel] = true

	info := c.info(channel)

	err = c.app.addSub(channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		return resp, ErrInternalServerError
	}

	if chOpts.Presence {
		err = c.app.addPresence(channel, c.UID, info)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, ErrInternalServerError
		}
	}

	if chOpts.Recover {
		if cmd.Recover {
			// Client provided subscribe request with recover flag on. Try to recover missed messages
			// automatically from history (we suppose here that history configured wisely) based on
			// provided last message id value.
			messages, err := c.app.History(channel)
			if err != nil {
				logger.ERROR.Printf("can't recover messages for channel %s: %s", string(channel), err)
				body.Messages = []Message{}
			} else {
				recoveredMessages, recovered := recoverMessages(cmd.Last, messages)
				body.Messages = recoveredMessages
				body.Recovered = recovered
			}
		} else {
			// Client don't want to recover messages yet, we just return last message id to him here.
			lastMessageID, err := c.app.lastMessageID(channel)
			if err != nil {
				logger.ERROR.Println(err)
			} else {
				body.Last = lastMessageID
			}
		}
	}

	if chOpts.JoinLeave {
		go func() {
			err = c.app.pubJoinLeave(channel, "join", info)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}()
	}

	if c.app.mediator != nil {
		c.app.mediator.Subscribe(channel, c.UID, c.User)
	}

	body.Status = true

	return resp, nil
}

// unsubscribeCmd handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *client) unsubscribeCmd(cmd *UnsubscribeClientCommand) (*clientResponse, error) {

	resp := newClientResponse("unsubscribe")

	channel := cmd.Channel
	if channel == "" {
		return nil, ErrInvalidMessage
	}

	body := &UnsubscribeBody{
		Channel: channel,
	}
	resp.Body = body

	chOpts, err := c.app.channelOpts(channel)
	if err != nil {
		resp.Err(clientError{err, errorAdviceFix})
		return resp, nil
	}

	info := c.info(channel)

	_, ok := c.Channels[channel]
	if ok {

		delete(c.Channels, channel)

		err = c.app.removePresence(channel, c.UID)
		if err != nil {
			logger.ERROR.Println(err)
		}

		if chOpts.JoinLeave {
			err = c.app.pubJoinLeave(channel, "leave", info)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}

		err = c.app.removeSub(channel, c)
		if err != nil {
			logger.ERROR.Println(err)
			resp.Err(clientError{ErrInternalServerError, errorAdviceNone})
			return resp, nil
		}

		if c.app.mediator != nil {
			c.app.mediator.Unsubscribe(channel, c.UID, c.User)
		}

	}

	body.Status = true

	return resp, nil
}

// publishCmd handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) publishCmd(cmd *PublishClientCommand) (*clientResponse, error) {

	resp := newClientResponse("publish")

	channel := cmd.Channel
	data := cmd.Data

	body := &PublishBody{
		Channel: channel,
	}
	resp.Body = body

	if _, ok := c.Channels[channel]; !ok {
		resp.Err(clientError{ErrPermissionDenied, errorAdviceFix})
		return resp, nil
	}

	info := c.info(channel)

	err := c.app.publish(channel, data, c.UID, &info, true)
	if err != nil {
		resp.Err(clientError{err, errorAdviceRetry})
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
func (c *client) presenceCmd(cmd *PresenceClientCommand) (*clientResponse, error) {

	resp := newClientResponse("presence")

	channel := cmd.Channel

	body := &PresenceBody{
		Channel: channel,
	}

	resp.Body = body

	if _, ok := c.Channels[channel]; !ok {
		resp.Err(clientError{ErrPermissionDenied, errorAdviceFix})
		return resp, nil
	}

	presence, err := c.app.Presence(channel)
	if err != nil {
		resp.Err(clientError{err, errorAdviceRetry})
		return resp, nil
	}

	body.Data = presence

	return resp, nil
}

// historyCmd handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag)
func (c *client) historyCmd(cmd *HistoryClientCommand) (*clientResponse, error) {

	resp := newClientResponse("history")

	channel := cmd.Channel

	body := &HistoryBody{
		Channel: channel,
	}

	resp.Body = body

	if _, ok := c.Channels[channel]; !ok {
		resp.Err(clientError{ErrPermissionDenied, errorAdviceFix})
		return resp, nil
	}

	history, err := c.app.History(channel)
	if err != nil {
		resp.Err(clientError{err, errorAdviceRetry})
		return resp, nil
	}

	body.Data = history

	return resp, nil
}
