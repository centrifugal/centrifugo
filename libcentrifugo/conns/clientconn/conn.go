package clientconn

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"github.com/centrifugal/centrifugo/libcentrifugo/bytequeue"
	"github.com/centrifugal/centrifugo/libcentrifugo/conns"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/metrics"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
	"github.com/centrifugal/centrifugo/libcentrifugo/raw"
	"github.com/satori/go.uuid"
)

func init() {
	metricsRegistry := plugin.Metrics

	metricsRegistry.RegisterCounter("client_num_msg_queued", metrics.NewCounter())
	metricsRegistry.RegisterCounter("client_num_msg_sent", metrics.NewCounter())
	metricsRegistry.RegisterCounter("client_num_msg_published", metrics.NewCounter())
	metricsRegistry.RegisterCounter("client_bytes_in", metrics.NewCounter())
	metricsRegistry.RegisterCounter("client_bytes_out", metrics.NewCounter())
	metricsRegistry.RegisterCounter("client_api_num_requests", metrics.NewCounter())
	metricsRegistry.RegisterCounter("client_num_connect", metrics.NewCounter())
	metricsRegistry.RegisterCounter("client_num_subscribe", metrics.NewCounter())

	quantiles := []float64{50, 90, 99, 99.99}
	var minValue int64 = 1        // record latencies in microseconds, min resolution 1mks.
	var maxValue int64 = 60000000 // record latencies in microseconds, max resolution 60s.
	numBuckets := 15              // histograms will be rotated every time we updating snapshot.
	sigfigs := 3
	metricsRegistry.RegisterHDRHistogram("client_api", metrics.NewHDRHistogram(numBuckets, minValue, maxValue, sigfigs, quantiles, "microseconds"))
}

// client represents client connection to Centrifugo - at moment this can be Websocket
// or SockJS connection. It abstracts away protocol of incoming connection having
// session interface. Session allows to Send messages via connection and to Close connection.
type client struct {
	sync.RWMutex
	node           *node.Node
	sess           conns.Session
	uid            string
	user           string
	timestamp      int64
	authenticated  bool
	defaultInfo    raw.Raw
	channelInfo    map[string]raw.Raw
	channels       map[string]struct{}
	messages       bytequeue.ByteQueue
	closeCh        chan struct{}
	closed         bool
	staleTimer     *time.Timer
	expireTimer    *time.Timer
	presenceTimer  *time.Timer
	sendTimeout    time.Duration
	maxQueueSize   int
	maxRequestSize int
	sendFinished   chan struct{}
}

var (
	arrayJSONPrefix  byte = '['
	objectJSONPrefix byte = '{'
)

func clientCommandsFromJSON(msgBytes []byte) ([]proto.ClientCommand, error) {
	var cmds []proto.ClientCommand
	firstByte := msgBytes[0]
	switch firstByte {
	case objectJSONPrefix:
		// single command request
		var cmd proto.ClientCommand
		err := json.Unmarshal(msgBytes, &cmd)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	case arrayJSONPrefix:
		// array of commands received
		err := json.Unmarshal(msgBytes, &cmds)
		if err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

// New creates new client connection.
func New(n *node.Node, s conns.Session) (conns.ClientConn, error) {
	config := n.Config()
	staleCloseDelay := config.StaleConnectionCloseDelay
	queueInitialCapacity := config.ClientQueueInitialCapacity
	maxQueueSize := config.ClientQueueMaxSize
	maxRequestSize := config.ClientRequestMaxSize

	c := client{
		uid:            uuid.NewV4().String(),
		node:           n,
		sess:           s,
		closeCh:        make(chan struct{}),
		messages:       bytequeue.New(queueInitialCapacity),
		maxQueueSize:   maxQueueSize,
		maxRequestSize: maxRequestSize,
		sendFinished:   make(chan struct{}),
	}
	go c.sendMessages()
	if staleCloseDelay > 0 {
		c.staleTimer = time.AfterFunc(staleCloseDelay, c.closeUnauthenticated)
	}
	return &c, nil
}

// sendMessages waits for messages from queue and sends them to client.
func (c *client) sendMessages() {
	defer close(c.sendFinished)
	for {
		msg, ok := c.messages.Wait()
		if !ok {
			if c.messages.Closed() {
				return
			}
			continue
		}
		err := c.sendMessage(msg)
		if err != nil {
			// Close in goroutine to let this function return.
			go c.Close(&conns.DisconnectAdvice{Reason: "error sending message", Reconnect: true})
			return
		}
		plugin.Metrics.Counters.Inc("client_num_msg_sent")
		plugin.Metrics.Counters.Add("client_bytes_out", int64(len(msg)))
	}
}

func (c *client) sendMessage(msg []byte) error {
	// Write timeout must be implemented inside session Send method.
	// Slow client connections will be closed eventually anyway after
	// exceeding client max queue size.
	return c.sess.Send(msg)
}

// closeUnauthenticated closes connection if it's not authenticated yet.
// At moment used to close connections which have not sent valid connect command
// in a reasonable time interval after actually connected to Centrifugo.
func (c *client) closeUnauthenticated() {
	c.RLock()
	authenticated := c.authenticated
	closed := c.closed
	c.RUnlock()
	if !authenticated && !closed {
		c.Close(&conns.DisconnectAdvice{Reason: "stale", Reconnect: false})
	}
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect
func (c *client) updateChannelPresence(ch string) {
	chOpts, err := c.node.ChannelOpts(ch)
	if err != nil {
		return
	}
	if !chOpts.Presence {
		return
	}
	c.node.AddPresence(ch, c.uid, c.info(ch))
}

// updatePresence updates presence info for all client channels
func (c *client) updatePresence() {
	c.RLock()
	if c.closed {
		return
	}
	for _, channel := range c.Channels() {
		c.updateChannelPresence(channel)
	}
	c.RUnlock()
	c.Lock()
	c.addPresenceUpdate()
	c.Unlock()
}

// Lock must be held outside.
func (c *client) addPresenceUpdate() {
	if c.closed {
		return
	}
	config := c.node.Config()
	presenceInterval := config.PresencePingInterval
	c.presenceTimer = time.AfterFunc(presenceInterval, c.updatePresence)
}

// No lock here as uid set on client initialization and can not be changed - we
// only read this value after.
func (c *client) UID() string {
	return c.uid
}

// No lock here as User() can not be called before we set user value in connect command.
// After this we only read this value.
func (c *client) User() string {
	return c.user
}

func (c *client) Channels() []string {
	c.RLock()
	defer c.RUnlock()
	keys := make([]string, len(c.channels))
	i := 0
	for k := range c.channels {
		keys[i] = k
		i++
	}
	return keys
}

func (c *client) Unsubscribe(ch string) error {
	cmd := &proto.UnsubscribeClientCommand{
		Channel: ch,
	}
	c.Lock()
	if c.closed {
		c.Unlock()
		return nil
	}
	resp, err := c.unsubscribeCmd(cmd)
	if err != nil {
		c.Unlock()
		return err
	}
	respJSON, err := json.Marshal(resp)
	if err != nil {
		c.Unlock()
		return err
	}
	c.Unlock()
	return c.Send(respJSON)
}

func (c *client) Send(message []byte) error {
	ok := c.messages.Add(message)
	if !ok {
		return proto.ErrClientClosed
	}
	plugin.Metrics.Counters.Inc("client_num_msg_queued")
	if c.messages.Size() > c.maxQueueSize {
		// Close in goroutine to not block message broadcast.
		go c.Close(&conns.DisconnectAdvice{Reason: "slow", Reconnect: false})
		return proto.ErrClientClosed
	}
	return nil
}

// clean called when connection was closed to make different clean up
// actions for a client
func (c *client) Close(advice *conns.DisconnectAdvice) error {
	c.Lock()
	defer c.Unlock()

	if c.closed {
		return nil
	}

	close(c.closeCh)
	c.closed = true

	c.messages.Close()

	if len(c.channels) > 0 {
		// unsubscribe from all channels
		for channel := range c.channels {
			cmd := &proto.UnsubscribeClientCommand{
				Channel: channel,
			}
			_, err := c.unsubscribeCmd(cmd)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	if c.authenticated {
		err := c.node.RemoveClientConn(c)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	if c.expireTimer != nil {
		c.expireTimer.Stop()
	}

	if c.presenceTimer != nil {
		c.presenceTimer.Stop()
	}

	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}

	if c.authenticated && c.node.Mediator() != nil {
		c.node.Mediator().Disconnect(c.uid, c.user)
	}

	if advice != nil && advice.Reason != "" {
		logger.DEBUG.Printf("Closing connection %s (user %s): %s", c.uid, c.user, advice.Reason)
	}

	c.sess.Close(advice)

	return nil
}

func (c *client) info(ch string) proto.ClientInfo {
	defaultInfo := c.defaultInfo
	channelInfo, _ := c.channelInfo[ch]
	return *proto.NewClientInfo(c.user, c.uid, defaultInfo, channelInfo)
}

func (c *client) Handle(msg []byte) error {
	started := time.Now()
	defer func() {
		plugin.Metrics.HDRHistograms.RecordMicroseconds("client_api", time.Now().Sub(started))
	}()
	plugin.Metrics.Counters.Inc("client_api_num_requests")
	plugin.Metrics.Counters.Add("client_bytes_in", int64(len(msg)))

	if len(msg) == 0 {
		logger.ERROR.Println("empty client request received")
		c.Close(&conns.DisconnectAdvice{Reason: proto.ErrInvalidMessage.Error(), Reconnect: false})
		return proto.ErrInvalidMessage
	} else if len(msg) > c.maxRequestSize {
		logger.ERROR.Println("client request exceeds max request size limit")
		c.Close(&conns.DisconnectAdvice{Reason: proto.ErrLimitExceeded.Error(), Reconnect: false})
		return proto.ErrLimitExceeded
	}

	commands, err := clientCommandsFromJSON(msg)
	if err != nil {
		logger.ERROR.Printf("Error unmarshaling message: %v", err)
		c.Close(&conns.DisconnectAdvice{Reason: proto.ErrInvalidMessage.Error(), Reconnect: false})
		return proto.ErrInvalidMessage
	}

	if len(commands) == 0 {
		// Nothing to do - in normal workflow such commands should never come.
		// Let's be strict here to prevent client sending useless messages.
		logger.ERROR.Println("got request from client without commands")
		c.Close(&conns.DisconnectAdvice{Reason: proto.ErrInvalidMessage.Error(), Reconnect: false})
		return proto.ErrInvalidMessage
	}

	err = c.handleCommands(commands)
	if err != nil {
		reconnect := false
		if err == proto.ErrInternalServerError {
			// In case of any internal server error we give client an advice to reconnect.
			// Any other error results in disconnect without reconnect.
			reconnect = true
		}
		c.Close(&conns.DisconnectAdvice{Reason: err.Error(), Reconnect: reconnect})
		return err
	}
	return nil
}

func (c *client) handleCommands(cmds []proto.ClientCommand) error {
	var err error
	mr := make(proto.MultiClientResponse, len(cmds))
	for i, command := range cmds {
		resp, err := c.handleCmd(command)
		if err != nil {
			return err
		}
		resp.SetUID(command.UID)
		mr[i] = resp
	}
	var jsonResp []byte
	if len(cmds) == 1 {
		jsonResp, err = json.Marshal(mr[0])
	} else {
		jsonResp, err = json.Marshal(mr)
	}
	if err != nil {
		logger.ERROR.Println(err)
		return proto.ErrInvalidMessage
	}
	err = c.Send(jsonResp)
	return err
}

// handleCmd dispatches clientCommand into correct command handler
func (c *client) handleCmd(command proto.ClientCommand) (proto.Response, error) {

	c.Lock()
	defer c.Unlock()

	if c.closed {
		return nil, proto.ErrClientClosed
	}

	var err error
	var resp proto.Response

	method := command.Method
	params := command.Params

	if method != "connect" && !c.authenticated {
		return nil, proto.ErrUnauthorized
	}

	switch method {
	case "connect":
		var cmd proto.ConnectClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, proto.ErrInvalidMessage
		}
		resp, err = c.connectCmd(&cmd)
	case "refresh":
		var cmd proto.RefreshClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, proto.ErrInvalidMessage
		}
		resp, err = c.refreshCmd(&cmd)
	case "subscribe":
		var cmd proto.SubscribeClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, proto.ErrInvalidMessage
		}
		resp, err = c.subscribeCmd(&cmd)
	case "unsubscribe":
		var cmd proto.UnsubscribeClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, proto.ErrInvalidMessage
		}
		resp, err = c.unsubscribeCmd(&cmd)
	case "publish":
		var cmd proto.PublishClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, proto.ErrInvalidMessage
		}
		resp, err = c.publishCmd(&cmd)
	case "ping":
		var cmd proto.PingClientCommand
		if len(params) > 0 {
			err = json.Unmarshal(params, &cmd)
			if err != nil {
				return nil, proto.ErrInvalidMessage
			}
		}
		resp, err = c.pingCmd(&cmd)
	case "presence":
		var cmd proto.PresenceClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, proto.ErrInvalidMessage
		}
		resp, err = c.presenceCmd(&cmd)
	case "history":
		var cmd proto.HistoryClientCommand
		err = json.Unmarshal(params, &cmd)
		if err != nil {
			return nil, proto.ErrInvalidMessage
		}
		resp, err = c.historyCmd(&cmd)
	default:
		return nil, proto.ErrMethodNotFound
	}
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// pingCmd handles ping command from client - this is necessary sometimes
// for example Heroku closes websocket connection after 55 seconds
// of inactive period when no messages with payload travelled over wire
func (c *client) pingCmd(cmd *proto.PingClientCommand) (proto.Response, error) {
	var body *proto.PingBody
	if cmd.Data != "" {
		body = &proto.PingBody{
			Data: cmd.Data,
		}
	}
	resp := proto.NewClientPingResponse(body)
	return resp, nil
}

func (c *client) expire() {
	config := c.node.Config()
	connLifetime := config.ConnLifetime

	if connLifetime <= 0 {
		return
	}

	c.RLock()
	timeToExpire := c.timestamp + connLifetime - time.Now().Unix()
	c.RUnlock()
	if timeToExpire > 0 {
		// connection was successfully refreshed.
		return
	}

	c.Close(&conns.DisconnectAdvice{Reason: "expired", Reconnect: true})
	return
}

// connectCmd handles connect command from client - client must send this
// command immediately after establishing Websocket or SockJS connection with
// Centrifugo
func (c *client) connectCmd(cmd *proto.ConnectClientCommand) (proto.Response, error) {

	plugin.Metrics.Counters.Inc("client_num_connect")

	if c.authenticated {
		logger.ERROR.Println("connect error: client already authenticated")
		return nil, proto.ErrInvalidMessage
	}

	user := cmd.User
	info := cmd.Info

	config := c.node.Config()

	secret := config.Secret
	insecure := config.Insecure
	closeDelay := config.ExpiredConnectionCloseDelay
	connLifetime := config.ConnLifetime
	version := c.node.Version()
	userConnectionLimit := config.UserConnectionLimit

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
			return nil, proto.ErrInvalidToken
		}
		ts, err := strconv.Atoi(timestamp)
		if err != nil {
			logger.ERROR.Println(err)
			return nil, proto.ErrInvalidMessage
		}
		c.timestamp = int64(ts)
	} else {
		c.timestamp = time.Now().Unix()
	}

	if userConnectionLimit > 0 && user != "" && len(c.node.ClientHub().UserConnections(user)) >= userConnectionLimit {
		logger.ERROR.Printf("limit of connections %d for user %s reached", userConnectionLimit, user)
		return nil, proto.ErrLimitExceeded
	}

	if c.node.Mediator() != nil {
		pass := c.node.Mediator().Connect(c.uid, c.user)
		if !pass {
			return nil, proto.ErrPermissionDenied
		}
	}

	c.user = user

	body := proto.ConnectBody{}
	body.Version = version
	body.Expires = connLifetime > 0
	body.TTL = connLifetime

	var timeToExpire int64

	if connLifetime > 0 && !insecure {
		timeToExpire = c.timestamp + connLifetime - time.Now().Unix()
		if timeToExpire <= 0 {
			body.Expired = true
			return proto.NewClientConnectResponse(body), nil
		}
	}

	c.authenticated = true
	if len(info) > 0 {
		c.defaultInfo = raw.Raw(info)
	}
	c.channels = map[string]struct{}{}
	c.channelInfo = map[string]raw.Raw{}

	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}

	c.addPresenceUpdate()

	err := c.node.AddClientConn(c)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, proto.ErrInternalServerError
	}

	if timeToExpire > 0 {
		duration := closeDelay + time.Duration(timeToExpire)*time.Second
		c.expireTimer = time.AfterFunc(duration, c.expire)
	}

	body.Client = c.uid
	return proto.NewClientConnectResponse(body), nil
}

// refreshCmd handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime option set.
func (c *client) refreshCmd(cmd *proto.RefreshClientCommand) (proto.Response, error) {

	user := cmd.User
	info := cmd.Info
	timestamp := cmd.Timestamp
	token := cmd.Token

	config := c.node.Config()
	secret := config.Secret

	isValid := auth.CheckClientToken(secret, string(user), timestamp, info, token)
	if !isValid {
		logger.ERROR.Println("invalid refresh token for user", user)
		return nil, proto.ErrInvalidToken
	}

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		logger.ERROR.Println(err)
		return nil, proto.ErrInvalidMessage
	}

	closeDelay := config.ExpiredConnectionCloseDelay
	connLifetime := config.ConnLifetime
	version := c.node.Version()

	body := proto.ConnectBody{}
	body.Version = version
	body.Expires = connLifetime > 0
	body.TTL = connLifetime
	body.Client = c.uid

	if connLifetime > 0 {
		// connection check enabled
		timeToExpire := int64(ts) + connLifetime - time.Now().Unix()
		if timeToExpire > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.timestamp = int64(ts)
			c.defaultInfo = raw.Raw(info)
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(timeToExpire)*time.Second + closeDelay
			c.expireTimer = time.AfterFunc(duration, c.expire)
		} else {
			body.Expired = true
		}
	}
	return proto.NewClientRefreshResponse(body), nil
}

func recoverMessages(last string, messages []proto.Message) ([]proto.Message, bool) {
	if last == "" {
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
func (c *client) subscribeCmd(cmd *proto.SubscribeClientCommand) (proto.Response, error) {

	plugin.Metrics.Counters.Inc("client_num_subscribe")

	channel := cmd.Channel
	if channel == "" {
		return nil, proto.ErrInvalidMessage
	}

	config := c.node.Config()
	secret := config.Secret
	maxChannelLength := config.MaxChannelLength
	channelLimit := config.ClientChannelLimit
	insecure := config.Insecure

	body := proto.SubscribeBody{
		Channel: channel,
	}

	if len(channel) > maxChannelLength {
		logger.ERROR.Printf("channel too long: max %d, got %d", maxChannelLength, len(channel))
		resp := proto.NewClientSubscribeResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrLimitExceeded, proto.ErrorAdviceFix})
		return resp, nil
	}

	if len(c.channels) >= channelLimit {
		logger.ERROR.Printf("maximum limit of channels per client reached: %d", channelLimit)
		resp := proto.NewClientSubscribeResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrLimitExceeded, proto.ErrorAdviceFix})
		return resp, nil
	}

	if _, ok := c.channels[channel]; ok {
		resp := proto.NewClientSubscribeResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrAlreadySubscribed, proto.ErrorAdviceFix})
		return resp, nil
	}

	if !c.node.UserAllowed(channel, c.user) || !c.node.ClientAllowed(channel, c.uid) {
		resp := proto.NewClientSubscribeResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
		return resp, nil
	}

	chOpts, err := c.node.ChannelOpts(channel)
	if err != nil {
		resp := proto.NewClientSubscribeResponse(body)
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceFix})
		return resp, nil
	}

	if !chOpts.Anonymous && c.user == "" && !insecure {
		resp := proto.NewClientSubscribeResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
		return resp, nil
	}

	if c.node.PrivateChannel(channel) {
		// private channel - subscription must be properly signed
		if string(c.uid) != string(cmd.Client) {
			resp := proto.NewClientSubscribeResponse(body)
			resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
			return resp, nil
		}
		isValid := auth.CheckChannelSign(secret, string(cmd.Client), string(channel), cmd.Info, cmd.Sign)
		if !isValid {
			resp := proto.NewClientSubscribeResponse(body)
			resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
			return resp, nil
		}
		if len(cmd.Info) > 0 {
			c.channelInfo[channel] = raw.Raw(cmd.Info)
		}
	}

	if c.node.Mediator() != nil {
		pass := c.node.Mediator().Subscribe(channel, c.uid, c.user)
		if !pass {
			resp := proto.NewClientSubscribeResponse(body)
			resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
			return resp, nil
		}
	}

	c.channels[channel] = struct{}{}

	err = c.node.AddClientSub(channel, c)
	if err != nil {
		logger.ERROR.Println(err)
		resp := proto.NewClientSubscribeResponse(body)
		return resp, proto.ErrInternalServerError
	}

	info := c.info(channel)

	if chOpts.Presence {
		err = c.node.AddPresence(channel, c.uid, info)
		if err != nil {
			logger.ERROR.Println(err)
			resp := proto.NewClientSubscribeResponse(body)
			return resp, proto.ErrInternalServerError
		}
	}

	if chOpts.Recover {
		if cmd.Recover {
			// Client provided subscribe request with recover flag on. Try to recover missed messages
			// automatically from history (we suppose here that history configured wisely) based on
			// provided last message id value.
			messages, err := c.node.History(channel)
			if err != nil {
				logger.ERROR.Printf("can't recover messages for channel %s: %s", string(channel), err)
				body.Messages = []proto.Message{}
			} else {
				recoveredMessages, recovered := recoverMessages(cmd.Last, messages)
				body.Messages = recoveredMessages
				body.Recovered = recovered
			}
		} else {
			// Client don't want to recover messages yet, we just return last message id to him here.
			lastMessageID, err := c.node.LastMessageID(channel)
			if err != nil {
				logger.ERROR.Println(err)
			} else {
				body.Last = lastMessageID
			}
		}
	}

	if chOpts.JoinLeave {
		c.node.PublishJoin(proto.NewJoinMessage(channel, info), &chOpts)
	}

	body.Status = true

	return proto.NewClientSubscribeResponse(body), nil
}

// unsubscribeCmd handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *client) unsubscribeCmd(cmd *proto.UnsubscribeClientCommand) (proto.Response, error) {

	channel := cmd.Channel
	if channel == "" {
		return nil, proto.ErrInvalidMessage
	}

	body := proto.UnsubscribeBody{
		Channel: channel,
	}

	chOpts, err := c.node.ChannelOpts(channel)
	if err != nil {
		resp := proto.NewClientUnsubscribeResponse(body)
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceFix})
		return resp, nil
	}

	info := c.info(channel)

	_, ok := c.channels[channel]
	if ok {

		delete(c.channels, channel)

		if chOpts.Presence {
			err = c.node.RemovePresence(channel, c.uid)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}

		if chOpts.JoinLeave {
			c.node.PublishLeave(proto.NewLeaveMessage(channel, info), &chOpts)
		}

		err = c.node.RemoveClientSub(channel, c)
		if err != nil {
			logger.ERROR.Println(err)
			resp := proto.NewClientUnsubscribeResponse(body)
			resp.SetErr(proto.ResponseError{proto.ErrInternalServerError, proto.ErrorAdviceNone})
			return resp, nil
		}

		if c.node.Mediator() != nil {
			c.node.Mediator().Unsubscribe(channel, c.uid, c.user)
		}

	}

	body.Status = true

	return proto.NewClientUnsubscribeResponse(body), nil
}

// publishCmd handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) publishCmd(cmd *proto.PublishClientCommand) (proto.Response, error) {

	channel := cmd.Channel
	data := cmd.Data

	body := proto.PublishBody{
		Channel: channel,
	}

	if string(channel) == "" || len(data) == 0 {
		resp := proto.NewClientPublishResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrInvalidMessage, proto.ErrorAdviceFix})
		return resp, nil
	}

	if _, ok := c.channels[channel]; !ok {
		resp := proto.NewClientPublishResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
		return resp, nil
	}

	info := c.info(channel)

	chOpts, err := c.node.ChannelOpts(channel)
	if err != nil {
		logger.ERROR.Println(err)
		resp := proto.NewClientPublishResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrInternalServerError, proto.ErrorAdviceRetry})
		return resp, nil
	}

	insecure := c.node.Config().Insecure

	if !chOpts.Publish && !insecure {
		resp := proto.NewClientPublishResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
		return resp, nil
	}

	if c.node.Mediator() != nil {
		// If mediator is set then we don't need to publish message
		// immediately as mediator will decide itself what to do with it.
		pass := c.node.Mediator().Message(channel, data, c.uid, &info)
		if !pass {
			resp := proto.NewClientPublishResponse(body)
			resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
			return resp, nil
		}
	}

	plugin.Metrics.Counters.Inc("client_num_msg_published")

	message := proto.NewMessage(channel, data, c.uid, &info)
	if chOpts.Watch {
		byteMessage, err := json.Marshal(message)
		if err != nil {
			logger.ERROR.Println(err)
		} else {
			c.node.PublishAdmin(proto.NewAdminMessage("message", byteMessage))
		}
	}

	err = <-c.node.Publish(message, &chOpts)
	if err != nil {
		resp := proto.NewClientPublishResponse(body)
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceRetry})
		return resp, nil
	}

	// message successfully published to engine.
	body.Status = true

	return proto.NewClientPublishResponse(body), nil
}

// presenceCmd handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *client) presenceCmd(cmd *proto.PresenceClientCommand) (proto.Response, error) {

	channel := cmd.Channel

	body := proto.PresenceBody{
		Channel: channel,
	}

	if _, ok := c.channels[channel]; !ok {
		resp := proto.NewClientPresenceResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
		return resp, nil
	}

	presence, err := c.node.Presence(channel)
	if err != nil {
		resp := proto.NewClientPresenceResponse(body)
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceRetry})
		return resp, nil
	}

	body.Data = presence

	return proto.NewClientPresenceResponse(body), nil
}

// historyCmd handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag)
func (c *client) historyCmd(cmd *proto.HistoryClientCommand) (proto.Response, error) {

	channel := cmd.Channel

	body := proto.HistoryBody{
		Channel: channel,
	}

	if _, ok := c.channels[channel]; !ok {
		resp := proto.NewClientHistoryResponse(body)
		resp.SetErr(proto.ResponseError{proto.ErrPermissionDenied, proto.ErrorAdviceFix})
		return resp, nil
	}

	history, err := c.node.History(channel)
	if err != nil {
		resp := proto.NewClientHistoryResponse(body)
		resp.SetErr(proto.ResponseError{err, proto.ErrorAdviceRetry})
		return resp, nil
	}

	body.Data = history

	return proto.NewClientHistoryResponse(body), nil
}
