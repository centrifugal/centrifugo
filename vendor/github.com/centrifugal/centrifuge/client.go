package centrifuge

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/bufpool"
	"github.com/centrifugal/centrifuge/internal/clientproto"
	"github.com/centrifugal/centrifuge/internal/prepared"
	"github.com/centrifugal/centrifuge/internal/recovery"

	"github.com/centrifugal/protocol"
	"github.com/google/uuid"
)

// clientEventHub allows to deal with client event handlers.
// All its methods are not goroutine-safe and supposed to be called
// once inside Node ConnectHandler.
type clientEventHub struct {
	aliveHandler         AliveHandler
	disconnectHandler    DisconnectHandler
	subscribeHandler     SubscribeHandler
	unsubscribeHandler   UnsubscribeHandler
	publishHandler       PublishHandler
	refreshHandler       RefreshHandler
	subRefreshHandler    SubRefreshHandler
	rpcHandler           RPCHandler
	messageHandler       MessageHandler
	presenceHandler      PresenceHandler
	presenceStatsHandler PresenceStatsHandler
	historyHandler       HistoryHandler
}

// OnAlive allows setting AliveHandler.
// AliveHandler called periodically for active client connection.
func (c *Client) OnAlive(h AliveHandler) {
	c.eventHub.aliveHandler = h
}

// OnRefresh allows setting RefreshHandler.
// RefreshHandler called when it's time to refresh expiring client connection.
func (c *Client) OnRefresh(h RefreshHandler) {
	c.eventHub.refreshHandler = h
}

// OnDisconnect allows setting DisconnectHandler.
// DisconnectHandler called when client disconnected.
func (c *Client) OnDisconnect(h DisconnectHandler) {
	c.eventHub.disconnectHandler = h
}

// OnMessage allows setting MessageHandler.
// MessageHandler called when client sent asynchronous message.
func (c *Client) OnMessage(h MessageHandler) {
	c.eventHub.messageHandler = h
}

// OnRPC allows setting RPCHandler.
// RPCHandler will be executed on every incoming RPC call.
func (c *Client) OnRPC(h RPCHandler) {
	c.eventHub.rpcHandler = h
}

// OnSubRefresh allows setting SubRefreshHandler.
// SubRefreshHandler called when it's time to refresh client subscription.
func (c *Client) OnSubRefresh(h SubRefreshHandler) {
	c.eventHub.subRefreshHandler = h
}

// OnSubscribe allows setting SubscribeHandler.
// SubscribeHandler called when client subscribes on channel.
func (c *Client) OnSubscribe(h SubscribeHandler) {
	c.eventHub.subscribeHandler = h
}

// OnUnsubscribe allows setting UnsubscribeHandler.
// UnsubscribeHandler called when client unsubscribes from channel.
func (c *Client) OnUnsubscribe(h UnsubscribeHandler) {
	c.eventHub.unsubscribeHandler = h
}

// OnPublish allows setting PublishHandler.
// PublishHandler called when client publishes message into channel.
func (c *Client) OnPublish(h PublishHandler) {
	c.eventHub.publishHandler = h
}

// OnPresence allows setting PresenceHandler.
// PresenceHandler called when Presence request from client received.
// At this moment you can only return a custom error or disconnect client.
func (c *Client) OnPresence(h PresenceHandler) {
	c.eventHub.presenceHandler = h
}

// OnPresenceStats allows settings PresenceStatsHandler.
// PresenceStatsHandler called when Presence Stats request from client received.
// At this moment you can only return a custom error or disconnect client.
func (c *Client) OnPresenceStats(h PresenceStatsHandler) {
	c.eventHub.presenceStatsHandler = h
}

// OnHistory allows settings HistoryHandler.
// HistoryHandler called when History request from client received.
// At this moment you can only return a custom error or disconnect client.
func (c *Client) OnHistory(h HistoryHandler) {
	c.eventHub.historyHandler = h
}

// We poll current position in channel from history storage periodically.
// If client position is wrong maxCheckPositionFailures times in a row
// then client will be disconnected with InsufficientState reason. Polling
// not used in channels with high frequency updates since we can check position
// comparing client offset with offset in incoming Publication.
const maxCheckPositionFailures uint8 = 2

// channelContext contains extra context for channel connection subscribed to.
// Note: this struct is aligned to consume less memory.
type channelContext struct {
	Info                  []byte
	expireAt              int64
	positionCheckTime     int64
	streamPosition        StreamPosition
	positionCheckFailures uint8
	serverSide            bool
	clientSideRefresh     bool
}

type timerOp uint8

const (
	timerOpStale    timerOp = 1
	timerOpPresence timerOp = 2
	timerOpExpire   timerOp = 3
)

type status uint8

const (
	statusConnecting status = 1
	statusConnected  status = 2
	statusClosed     status = 3
)

// Client represents client connection to server.
type Client struct {
	mu                sync.RWMutex
	connectMu         sync.Mutex // allows to sync connect with disconnect.
	presenceMu        sync.Mutex // allows to sync presence routine with client closing.
	ctx               context.Context
	transport         Transport
	node              *Node
	exp               int64
	publications      *pubQueue
	channels          map[string]channelContext
	messageWriter     *writer
	pubSubSync        *recovery.PubSubSync
	uid               string
	user              string
	info              []byte
	publicationsOnce  sync.Once
	authenticated     bool
	clientSideRefresh bool
	status            status
	timerOp           timerOp
	nextPresence      int64
	nextExpire        int64
	eventHub          *clientEventHub
	timer             *time.Timer
}

// ClientCloseFunc must be called on Transport handler close to clean up Client.
type ClientCloseFunc func() error

// NewClient initializes new Client.
func NewClient(ctx context.Context, n *Node, t Transport) (*Client, ClientCloseFunc, error) {
	uuidObject, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, err
	}

	c := &Client{
		ctx:          ctx,
		uid:          uuidObject.String(),
		node:         n,
		transport:    t,
		channels:     make(map[string]channelContext),
		pubSubSync:   recovery.NewPubSubSync(),
		publications: newPubQueue(),
		status:       statusConnecting,
		eventHub:     &clientEventHub{},
	}

	messageWriterConf := writerConfig{
		MaxQueueSize: n.config.ClientQueueMaxSize,
		WriteFn: func(data []byte) error {
			if err := t.Write(data); err != nil {
				go func() { _ = c.close(DisconnectWriteError) }()
				return err
			}
			incTransportMessagesSent(t.Name())
			return nil
		},
		WriteManyFn: func(data ...[]byte) error {
			buf := bufpool.GetBuffer()
			for _, payload := range data {
				buf.Write(payload)
			}
			if err := t.Write(buf.Bytes()); err != nil {
				go func() { _ = c.close(DisconnectWriteError) }()
				bufpool.PutBuffer(buf)
				return err
			}
			bufpool.PutBuffer(buf)
			addTransportMessagesSent(t.Name(), float64(len(data)))
			return nil
		},
	}

	c.messageWriter = newWriter(messageWriterConf)
	go c.messageWriter.run()

	staleCloseDelay := n.config.ClientStaleCloseDelay
	if staleCloseDelay > 0 && !c.authenticated {
		c.mu.Lock()
		c.timerOp = timerOpStale
		c.timer = time.AfterFunc(staleCloseDelay, c.onTimerOp)
		c.mu.Unlock()
	}

	return c, func() error { return c.close(nil) }, nil
}

func (c *Client) onTimerOp() {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return
	}
	timerOp := c.timerOp
	c.mu.Unlock()
	switch timerOp {
	case timerOpStale:
		c.closeUnauthenticated()
	case timerOpPresence:
		c.updatePresence()
	case timerOpExpire:
		c.expire()
	}
}

// Lock must be held outside.
func (c *Client) scheduleNextTimer() {
	if c.status == statusClosed {
		return
	}
	c.stopTimer()
	var minEventTime int64
	var nextTimerOp timerOp
	var needTimer bool
	if c.nextExpire > 0 {
		nextTimerOp = timerOpExpire
		minEventTime = c.nextExpire
		needTimer = true
	}
	if c.nextPresence > 0 && (minEventTime == 0 || c.nextPresence < minEventTime) {
		nextTimerOp = timerOpPresence
		minEventTime = c.nextPresence
		needTimer = true
	}
	if needTimer {
		c.timerOp = nextTimerOp
		afterDuration := time.Duration(minEventTime-time.Now().UnixNano()) * time.Nanosecond
		c.timer = time.AfterFunc(afterDuration, c.onTimerOp)
	}
}

// Lock must be held outside.
func (c *Client) stopTimer() {
	if c.timer != nil {
		c.timer.Stop()
	}
}

// Lock must be held outside.
func (c *Client) addPresenceUpdate() {
	config := c.node.config
	presenceInterval := config.ClientPresenceUpdateInterval
	c.nextPresence = time.Now().Add(presenceInterval).UnixNano()
	c.scheduleNextTimer()
}

// Lock must be held outside.
func (c *Client) addExpireUpdate(after time.Duration) {
	c.nextExpire = time.Now().Add(after).UnixNano()
	c.scheduleNextTimer()
}

// closeUnauthenticated closes connection if it's not authenticated yet.
// At moment used to close client connections which have not sent valid
// connect command in a reasonable time interval after established connection
// with server.
func (c *Client) closeUnauthenticated() {
	c.mu.RLock()
	authenticated := c.authenticated
	closed := c.status == statusClosed
	c.mu.RUnlock()
	if !authenticated && !closed {
		_ = c.close(DisconnectStale)
	}
}

func (c *Client) transportEnqueue(reply *prepared.Reply) error {
	data := reply.Data()
	disconnect := c.messageWriter.enqueue(data)
	if disconnect != nil {
		// close in goroutine to not block message broadcast.
		go func() { _ = c.close(disconnect) }()
		return io.EOF
	}
	return nil
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect.
func (c *Client) updateChannelPresence(ch string) error {
	chOpts, found, err := c.node.channelOptions(ch)
	if err != nil || !found {
		return nil
	}
	if !chOpts.Presence {
		return nil
	}
	info := c.clientInfo(ch)
	return c.node.addPresence(ch, c.uid, info)
}

// Context returns client Context. This context will be canceled
// as soon as client connection closes.
func (c *Client) Context() context.Context {
	return c.ctx
}

func (c *Client) checkSubscriptionExpiration(channel string, channelContext channelContext, delay time.Duration, resultCB func(bool)) {
	now := c.node.nowTimeGetter().Unix()
	expireAt := channelContext.expireAt
	clientSideRefresh := channelContext.clientSideRefresh
	if expireAt > 0 && now > expireAt+int64(delay.Seconds()) {
		// Subscription expired.
		if clientSideRefresh || c.eventHub.subRefreshHandler == nil {
			// The only way subscription could be refreshed in this case is via
			// SUB_REFRESH command sent from client but looks like that command
			// with new refreshed token have not been received in configured window.
			resultCB(false)
			return
		}
		cb := func(reply SubRefreshReply, err error) {
			if err != nil {
				resultCB(false)
				return
			}
			if reply.Expired || (reply.ExpireAt > 0 && reply.ExpireAt < now) {
				resultCB(false)
				return
			}
			c.mu.Lock()
			if ctx, ok := c.channels[channel]; ok {
				if len(reply.Info) > 0 {
					ctx.Info = reply.Info
				}
				ctx.expireAt = reply.ExpireAt
				c.channels[channel] = ctx
			}
			c.mu.Unlock()
			resultCB(true)
		}
		// Give subscription a chance to be refreshed via SubRefreshHandler.
		event := SubRefreshEvent{Channel: channel}
		c.eventHub.subRefreshHandler(event, cb)
		return
	}
	resultCB(true)
}

// updatePresence used for various periodic actions we need to do with client connections.
func (c *Client) updatePresence() {
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	config := c.node.config
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return
	}
	channels := make(map[string]channelContext, len(c.channels))
	for channel, channelContext := range c.channels {
		channels[channel] = channelContext
	}
	c.mu.Unlock()
	if c.eventHub.aliveHandler != nil {
		c.eventHub.aliveHandler()
	}
	for channel, channelContext := range channels {
		c.checkSubscriptionExpiration(channel, channelContext, config.ClientExpiredSubCloseDelay, func(result bool) {
			// Ideally we should deal with single expired subscription in this
			// case - i.e. unsubscribe client from channel and give an advice
			// to resubscribe. But there is scenario when browser goes online
			// after computer was in sleeping mode which I have not managed to
			// handle reliably on client side when unsubscribe with resubscribe
			// flag was used. So I decided to stick with disconnect for now -
			// it seems to work fine and drastically simplifies client code.
			if !result {
				go func() { _ = c.close(DisconnectSubExpired) }()
			}
		})

		checkDelay := config.ClientChannelPositionCheckDelay
		if checkDelay > 0 && !c.checkPosition(checkDelay, channel, channelContext) {
			go func() { _ = c.close(DisconnectInsufficientState) }()
			// No need to proceed after close.
			return
		}

		err := c.updateChannelPresence(channel)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error updating presence for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		}
	}
	c.mu.Lock()
	c.addPresenceUpdate()
	c.mu.Unlock()
}

func (c *Client) checkPosition(checkDelay time.Duration, ch string, chCtx channelContext) bool {
	chOpts, found, err := c.node.channelOptions(ch)
	if err != nil || !found {
		return true
	}
	if !chOpts.HistoryRecover {
		return true
	}
	nowUnix := c.node.nowTimeGetter().Unix()

	isInitialCheck := chCtx.positionCheckTime == 0
	isTimeToCheck := nowUnix-chCtx.positionCheckTime > int64(checkDelay.Seconds())
	needCheckPosition := isInitialCheck || isTimeToCheck

	if !needCheckPosition {
		return true
	}
	position := chCtx.streamPosition
	streamTop, err := c.node.streamTop(ch)
	if err != nil {
		return true
	}

	isValidPosition := streamTop.Offset == position.Offset && streamTop.Epoch == position.Epoch
	keepConnection := true
	c.mu.Lock()
	if chContext, ok := c.channels[ch]; ok {
		chContext.positionCheckTime = nowUnix
		if !isValidPosition {
			chContext.positionCheckFailures++
			keepConnection = chContext.positionCheckFailures < maxCheckPositionFailures
		} else {
			chContext.positionCheckFailures = 0
		}
		c.channels[ch] = chContext
	}
	c.mu.Unlock()
	return keepConnection
}

// ID returns unique client connection id.
func (c *Client) ID() string {
	return c.uid
}

// UserID returns user id associated with client connection.
func (c *Client) UserID() string {
	return c.user
}

// Transport returns client connection transport information.
func (c *Client) Transport() TransportInfo {
	return c.transport
}

// Channels returns a slice of channels client connection currently subscribed to.
func (c *Client) Channels() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	channels := make([]string, 0, len(c.channels))
	for ch := range c.channels {
		channels = append(channels, ch)
	}
	return channels
}

// IsSubscribed returns true if client subscribed to a channel.
func (c *Client) IsSubscribed(ch string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.channels[ch]
	return ok
}

// Send data to client. This sends an asynchronous message – data will be
// just written to connection. on client side this message can be handled
// with Message handler.
func (c *Client) Send(data []byte) error {
	p := &protocol.Message{
		Data: data,
	}

	pushEncoder := protocol.GetPushEncoder(c.transport.Protocol().toProto())
	data, err := pushEncoder.EncodeMessage(p)
	if err != nil {
		return err
	}
	result, err := pushEncoder.Encode(clientproto.NewMessagePush(data))
	if err != nil {
		return err
	}

	reply := prepared.NewReply(&protocol.Reply{
		Result: result,
	}, c.transport.Protocol().toProto())

	return c.transportEnqueue(reply)
}

// Unsubscribe allows to unsubscribe client from channel.
func (c *Client) Unsubscribe(ch string, opts ...UnsubscribeOption) error {
	unsubscribeOpts := &UnsubscribeOptions{}
	for _, opt := range opts {
		opt(unsubscribeOpts)
	}

	c.mu.RLock()
	if c.status == statusClosed {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	err := c.unsubscribe(ch)
	if err != nil {
		return err
	}
	return c.sendUnsub(ch, unsubscribeOpts.Resubscribe)
}

func (c *Client) sendUnsub(ch string, resubscribe bool) error {
	pushEncoder := protocol.GetPushEncoder(c.transport.Protocol().toProto())

	data, err := pushEncoder.EncodeUnsub(&protocol.Unsub{Resubscribe: resubscribe})
	if err != nil {
		return err
	}
	result, err := pushEncoder.Encode(clientproto.NewUnsubPush(ch, data))
	if err != nil {
		return err
	}

	reply := prepared.NewReply(&protocol.Reply{
		Result: result,
	}, c.transport.Protocol().toProto())

	_ = c.transportEnqueue(reply)

	return nil
}

// Disconnect client connection with specific disconnect code and reason.
// This method internally creates a new goroutine at moment to do
// closing stuff. An extra goroutine is required to solve disconnect
// and alive callback ordering/sync problems. Will be a noop if client
// already closed. As this method runs a separate goroutine client
// connection will be closed eventually (i.e. not immediately).
func (c *Client) Disconnect(disconnect *Disconnect) error {
	go func() {
		_ = c.close(disconnect)
	}()
	return nil
}

func (c *Client) close(disconnect *Disconnect) error {
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return nil
	}
	prevStatus := c.status
	c.status = statusClosed

	c.stopTimer()

	channels := make(map[string]channelContext, len(c.channels))
	for channel, channelContext := range c.channels {
		channels[channel] = channelContext
	}
	c.mu.Unlock()

	if len(channels) > 0 {
		// Unsubscribe from all channels.
		for channel := range channels {
			err := c.unsubscribe(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error unsubscribing client from channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			}
		}
	}

	c.mu.RLock()
	authenticated := c.authenticated
	c.mu.RUnlock()

	if authenticated {
		err := c.node.removeClient(c)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error removing client", map[string]interface{}{"user": c.user, "client": c.uid, "error": err.Error()}))
		}
	}

	// close writer and send messages remaining in writer queue if any.
	_ = c.messageWriter.close()

	c.publications.Close()

	_ = c.transport.Close(disconnect)

	if disconnect != nil && disconnect.Reason != "" {
		c.node.logger.log(newLogEntry(LogLevelDebug, "closing client connection", map[string]interface{}{"client": c.uid, "user": c.user, "reason": disconnect.Reason, "reconnect": disconnect.Reconnect}))
	}
	if disconnect != nil {
		incServerDisconnect(disconnect.Code)
	}
	if c.eventHub.disconnectHandler != nil && prevStatus == statusConnected {
		c.eventHub.disconnectHandler(DisconnectEvent{
			Disconnect: disconnect,
		})
	}
	return nil
}

// Lock must be held outside.
func (c *Client) clientInfo(ch string) *ClientInfo {
	var channelInfo protocol.Raw
	channelContext, ok := c.channels[ch]
	if ok {
		channelInfo = channelContext.Info
	}
	return &ClientInfo{
		UserID:   c.user,
		ClientID: c.uid,
		ConnInfo: c.info,
		ChanInfo: channelInfo,
	}
}

// Handle raw data encoded with Centrifuge protocol. Not goroutine-safe, supposed to be
// called only from transport reader.
func (c *Client) Handle(data []byte) bool {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()

	if len(data) == 0 {
		c.node.logger.log(newLogEntry(LogLevelError, "empty client request received", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
		go func() { _ = c.close(DisconnectBadRequest) }()
		return false
	}

	protoType := c.transport.Protocol().toProto()
	decoder := protocol.GetCommandDecoder(protoType, data)
	defer protocol.PutCommandDecoder(protoType, decoder)

	for {
		cmd, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding command", map[string]interface{}{"data": string(data), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
			go func() { _ = c.close(DisconnectBadRequest) }()
			return false
		}

		if cmd.Method != protocol.MethodTypeConnect && !c.authenticated {
			// Client must send connect command to authenticate itself first.
			c.node.logger.log(newLogEntry(LogLevelInfo, "client not authenticated to handle command", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "command": fmt.Sprintf("%v", cmd)}))
			go func() { _ = c.close(DisconnectBadRequest) }()
			return false
		}

		if cmd.ID == 0 && cmd.Method != protocol.MethodTypeSend {
			// Only send command from client can be sent without incremental ID.
			c.node.logger.log(newLogEntry(LogLevelInfo, "command ID required for commands with reply expected", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
			go func() { _ = c.close(DisconnectBadRequest) }()
			return false
		}

		select {
		case <-c.ctx.Done():
			return false
		default:
		}

		disconnect := c.handleCommand(cmd)

		select {
		case <-c.ctx.Done():
			return false
		default:
		}
		if disconnect != nil {
			if disconnect != DisconnectNormal {
				c.node.logger.log(newLogEntry(LogLevelInfo, "disconnect after handling command", map[string]interface{}{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			}
			go func() { _ = c.close(disconnect) }()
			return false
		}
	}
	return true
}

type replyWriter struct {
	write func(*protocol.Reply) error
	flush func() error
	done  func()
}

// handleCommand dispatches Command into correct command handler.
func (c *Client) handleCommand(cmd *protocol.Command) *Disconnect {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	method := cmd.Method
	params := cmd.Params

	protoType := c.transport.Protocol().toProto()
	encoder := protocol.GetReplyEncoder(protoType)

	var encodeErr error

	started := time.Now()

	write := func(rep *protocol.Reply) error {
		rep.ID = cmd.ID
		if rep.Error != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "client command error", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "error": rep.Error.Error()}))
			incReplyError(cmd.Method, rep.Error.Code)
		}

		encodeErr = encoder.Encode(rep)
		if encodeErr != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding reply", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "client": c.ID(), "user": c.UserID(), "error": encodeErr.Error()}))
		}
		return encodeErr
	}

	flush := func() error {
		buf := encoder.Finish()
		if len(buf) > 0 {
			disconnect := c.messageWriter.enqueue(buf)
			if disconnect != nil {
				if c.node.logger.enabled(LogLevelDebug) {
					c.node.logger.log(newLogEntry(LogLevelDebug, "disconnect after sending reply", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
				}
				go func() { _ = c.close(disconnect) }()
				return fmt.Errorf("flush error")
			}
		}
		encoder.Reset()
		return nil
	}

	// done should be called after command fully processed.
	done := func() {
		protocol.PutReplyEncoder(protoType, encoder)
		observeCommandDuration(method, time.Since(started))
	}

	// The rule is as follows: if command handler returns an
	// error then we handle it here: write error into connection
	// or return disconnect further to caller and call rw.done()
	// in the end.
	// If handler returned nil error then we assume that all
	// rw operations will be executed inside handler itself.
	rw := &replyWriter{write, flush, done}

	var handleErr error

	switch method {
	case protocol.MethodTypeConnect:
		handleErr = c.handleConnect(params, rw)
	case protocol.MethodTypePing:
		handleErr = c.handlePing(params, rw)
	case protocol.MethodTypeSubscribe:
		handleErr = c.handleSubscribe(params, rw)
	case protocol.MethodTypeUnsubscribe:
		handleErr = c.handleUnsubscribe(params, rw)
	case protocol.MethodTypePublish:
		handleErr = c.handlePublish(params, rw)
	case protocol.MethodTypePresence:
		handleErr = c.handlePresence(params, rw)
	case protocol.MethodTypePresenceStats:
		handleErr = c.handlePresenceStats(params, rw)
	case protocol.MethodTypeHistory:
		handleErr = c.handleHistory(params, rw)
	case protocol.MethodTypeRPC:
		handleErr = c.handleRPC(params, rw)
	case protocol.MethodTypeSend:
		handleErr = c.handleSend(params, rw)
	case protocol.MethodTypeRefresh:
		handleErr = c.handleRefresh(params, rw)
	case protocol.MethodTypeSubRefresh:
		handleErr = c.handleSubRefresh(params, rw)
	default:
		handleErr = ErrorMethodNotFound
	}
	if encodeErr != nil {
		return DisconnectServerError
	}
	if handleErr != nil {
		defer rw.done()
		switch t := handleErr.(type) {
		case *Disconnect:
			return t
		default:
			c.writeErrorFlush(rw, toClientErr(handleErr))
		}
	}
	return nil
}

func (c *Client) checkExpired() {
	c.mu.RLock()
	closed := c.status == statusClosed
	clientSideRefresh := c.clientSideRefresh
	exp := c.exp
	c.mu.RUnlock()
	if closed || exp == 0 {
		return
	}
	now := time.Now().Unix()
	ttl := exp - now

	if !clientSideRefresh && c.eventHub.refreshHandler != nil {
		if ttl > 0 {
			c.mu.Lock()
			if c.status != statusClosed {
				c.addExpireUpdate(time.Duration(ttl) * time.Second)
			}
			c.mu.Unlock()
		}
	}

	if ttl > 0 {
		// Connection was successfully refreshed.
		return
	}

	_ = c.close(DisconnectExpired)
}

func (c *Client) expire() {
	c.mu.RLock()
	closed := c.status == statusClosed
	clientSideRefresh := c.clientSideRefresh
	c.mu.RUnlock()
	if closed {
		return
	}
	if !clientSideRefresh && c.eventHub.refreshHandler != nil {
		cb := func(reply RefreshReply, err error) {
			if err != nil {
				switch t := err.(type) {
				case *Disconnect:
					_ = c.close(t)
					return
				default:
					_ = c.close(DisconnectServerError)
					return
				}
			}
			if reply.Expired {
				_ = c.close(DisconnectExpired)
				return
			}
			if reply.ExpireAt > 0 {
				c.mu.Lock()
				c.exp = reply.ExpireAt
				if reply.Info != nil {
					c.info = reply.Info
				}
				c.mu.Unlock()
			}
			c.checkExpired()
			return
		}
		c.eventHub.refreshHandler(RefreshEvent{}, cb)
	} else {
		c.checkExpired()
	}
}

func (c *Client) handleConnect(params protocol.Raw, rw *replyWriter) error {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeConnect(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding connect", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	disconnect := c.connectCmd(cmd, rw)
	if disconnect != nil {
		return disconnect
	}
	c.triggerConnect()
	c.scheduleOnConnectTimers()
	return nil
}

func (c *Client) triggerConnect() {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	if c.status != statusConnecting {
		return
	}
	if c.node.clientEvents.connectHandler == nil {
		c.status = statusConnected
		return
	}
	c.node.clientEvents.connectHandler(c)
	c.status = statusConnected
}

func (c *Client) scheduleOnConnectTimers() {
	// Make presence and refresh handlers always run after client connect event.
	c.mu.Lock()
	c.addPresenceUpdate()
	if c.exp > 0 {
		expireAfter := time.Duration(c.exp-time.Now().Unix()) * time.Second
		if c.clientSideRefresh {
			conf := c.node.config
			expireAfter += conf.ClientExpiredCloseDelay
		}
		c.addExpireUpdate(expireAfter)
	}
	c.mu.Unlock()
}

func (c *Client) handleRefresh(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.refreshHandler == nil {
		return ErrorNotAvailable
	}

	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding refresh", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	if cmd.Token == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client token required to refresh", map[string]interface{}{"user": c.user, "client": c.uid}))
		return DisconnectBadRequest
	}

	c.mu.RLock()
	clientSideRefresh := c.clientSideRefresh
	c.mu.RUnlock()

	if !clientSideRefresh {
		// Client not supposed to send refresh command in case of server-side refresh mechanism.
		return DisconnectBadRequest
	}

	event := RefreshEvent{
		ClientSideRefresh: true,
		Token:             cmd.Token,
	}

	cb := func(reply RefreshReply, err error) {
		defer rw.done()

		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}

		if reply.Expired {
			_ = c.Disconnect(DisconnectExpired)
			return
		}

		expireAt := reply.ExpireAt
		info := reply.Info

		res := &protocol.RefreshResult{
			Version: c.node.config.Version,
			Expires: expireAt > 0,
			Client:  c.uid,
		}

		ttl := expireAt - time.Now().Unix()

		if ttl > 0 {
			res.TTL = uint32(ttl)
		}

		if expireAt > 0 {
			// connection check enabled
			if ttl > 0 {
				// connection refreshed, update client timestamp and set new expiration timeout
				c.mu.Lock()
				c.exp = expireAt
				if len(info) > 0 {
					c.info = info
				}
				duration := time.Duration(ttl)*time.Second + c.node.config.ClientExpiredCloseDelay
				c.addExpireUpdate(duration)
				c.mu.Unlock()
			} else {
				c.writeErrorFlush(rw, ErrorExpired)
				return
			}
		}

		replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeRefreshResult(res)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding refresh", map[string]interface{}{"error": err.Error()}))
			c.writeErrorFlush(rw, ErrorInternal)
			return
		}

		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	c.eventHub.refreshHandler(event, cb)
	return nil
}

func (c *Client) handleSubscribe(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.subscribeHandler == nil {
		return ErrorNotAvailable
	}

	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeSubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding subscribe", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	chOpts, replyError, disconnect := c.validateSubscribeRequest(cmd)
	if disconnect != nil || replyError != nil {
		if disconnect != nil {
			return disconnect
		}
		return replyError
	}

	event := SubscribeEvent{
		Channel: cmd.Channel,
		Token:   cmd.Token,
	}

	cb := func(reply SubscribeReply, err error) {
		defer rw.done()

		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}

		ctx := c.subscribeCmd(cmd, chOpts, reply, rw, false)

		if ctx.disconnect != nil {
			_ = c.Disconnect(ctx.disconnect)
			return
		}
		if ctx.err != nil {
			c.writeDisconnectOrErrorFlush(rw, ctx.err)
			return
		}

		_ = rw.flush()

		if ctx.chOpts.JoinLeave && ctx.clientInfo != nil {
			go func() { _ = c.node.publishJoin(cmd.Channel, ctx.clientInfo) }()
		}
	}
	c.eventHub.subscribeHandler(event, cb)
	return nil
}

func (c *Client) handleSubRefresh(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.subRefreshHandler == nil {
		return ErrorNotAvailable
	}

	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeSubRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding sub refresh", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for sub refresh", map[string]interface{}{"user": c.user, "client": c.uid}))
		return DisconnectBadRequest
	}

	c.mu.RLock()
	ctx, okChannel := c.channels[channel]
	clientSideRefresh := ctx.clientSideRefresh
	c.mu.RUnlock()
	if !okChannel {
		// Must be subscribed to refresh subscription.
		return ErrorPermissionDenied
	}

	if !clientSideRefresh {
		// Client not supposed to send sub refresh command in case of server-side
		// subscription refresh mechanism.
		return DisconnectBadRequest
	}

	if cmd.Token == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "subscription refresh token required", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
		return ErrorBadRequest
	}

	event := SubRefreshEvent{
		ClientSideRefresh: true,
		Channel:           cmd.Channel,
		Token:             cmd.Token,
	}

	cb := func(reply SubRefreshReply, err error) {
		defer rw.done()

		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}

		res := &protocol.SubRefreshResult{}

		if reply.ExpireAt > 0 {
			res.Expires = true
			now := time.Now().Unix()
			if reply.ExpireAt < now {
				c.writeErrorFlush(rw, ErrorExpired)
				return
			}
			res.TTL = uint32(reply.ExpireAt - now)
		}

		c.mu.Lock()
		channelContext, okChan := c.channels[channel]
		if okChan {
			channelContext.Info = reply.Info
			channelContext.expireAt = reply.ExpireAt
		}
		c.channels[channel] = channelContext
		c.mu.Unlock()

		replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeSubRefreshResult(res)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding sub refresh", map[string]interface{}{"error": err.Error()}))
			c.writeErrorFlush(rw, ErrorInternal)
			return
		}
		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	c.eventHub.subRefreshHandler(event, cb)
	return nil
}

func (c *Client) handleUnsubscribe(params protocol.Raw, rw *replyWriter) error {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeUnsubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding unsubscribe", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for unsubscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return DisconnectBadRequest
	}

	if err := c.unsubscribe(channel); err != nil {
		return err
	}

	replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeUnsubscribeResult(&protocol.UnsubscribeResult{})
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error encoding unsubscribe", map[string]interface{}{"error": err.Error()}))
		return DisconnectServerError
	}

	_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	rw.done()
	return nil
}

func (c *Client) handlePublish(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.publishHandler == nil {
		return ErrorNotAvailable
	}

	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePublish(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding publish", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	channel := cmd.Channel
	data := cmd.Data

	if channel == "" || len(data) == 0 {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel and data required for publish", map[string]interface{}{"user": c.user, "client": c.uid}))
		return DisconnectBadRequest
	}

	c.mu.RLock()
	info := c.clientInfo(channel)
	c.mu.RUnlock()

	event := PublishEvent{
		Channel: channel,
		Data:    data,
		Info:    info,
	}

	cb := func(reply PublishReply, err error) {
		defer rw.done()

		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}

		publishResult := reply.Result

		if publishResult == nil {
			_, err := c.node.publish(channel, data, info)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error publishing", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				c.writeErrorFlush(rw, toClientErr(err))
				return
			}
		}

		replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodePublishResult(&protocol.PublishResult{})
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding publish", map[string]interface{}{"error": err.Error()}))
			c.writeErrorFlush(rw, ErrorInternal)
			return
		}
		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	c.eventHub.publishHandler(event, cb)
	return nil
}

func (c *Client) handlePresence(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.presenceHandler == nil {
		return ErrorNotAvailable
	}

	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePresence(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	channel := cmd.Channel
	if channel == "" {
		return DisconnectBadRequest
	}

	chOpts, found, err := c.node.channelOptions(channel)
	if err != nil {
		return err
	}
	if !found {
		return ErrorUnknownChannel
	}
	if !chOpts.Presence {
		return ErrorNotAvailable
	}

	event := PresenceEvent{
		Channel: channel,
	}

	cb := func(reply PresenceReply, err error) {
		defer rw.done()
		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}

		result, err := c.node.Presence(channel)
		if err != nil {
			c.node.logger.log(newLogEntry(errLogLevel(err), "error getting presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			c.writeErrorFlush(rw, toClientErr(err))
			return
		}

		presence := make(map[string]*protocol.ClientInfo, len(result.Presence))
		for k, v := range result.Presence {
			presence[k] = infoToProto(v)
		}

		replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodePresenceResult(&protocol.PresenceResult{
			Presence: presence,
		})
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence", map[string]interface{}{"error": err.Error()}))
			c.writeErrorFlush(rw, ErrorInternal)
			return
		}
		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	c.eventHub.presenceHandler(event, cb)
	return nil
}

func (c *Client) handlePresenceStats(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.presenceStatsHandler == nil {
		return ErrorNotAvailable
	}

	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePresenceStats(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence stats", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	channel := cmd.Channel
	if channel == "" {
		return DisconnectBadRequest
	}

	chOpts, found, err := c.node.channelOptions(channel)
	if err != nil {
		return err
	}
	if !found {
		return ErrorUnknownChannel
	}
	if !chOpts.Presence {
		return ErrorNotAvailable
	}

	event := PresenceStatsEvent{
		Channel: channel,
	}

	cb := func(reply PresenceStatsReply, err error) {
		defer rw.done()
		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}

		result, err := c.node.PresenceStats(channel)
		if err != nil {
			c.node.logger.log(newLogEntry(errLogLevel(err), "error getting presence stats", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			c.writeErrorFlush(rw, toClientErr(err))
			return
		}

		replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodePresenceStatsResult(&protocol.PresenceStatsResult{
			NumClients: uint32(result.NumClients),
			NumUsers:   uint32(result.NumUsers),
		})
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence stats", map[string]interface{}{"error": err.Error()}))
			c.writeErrorFlush(rw, ErrorInternal)
			return
		}
		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	c.eventHub.presenceStatsHandler(event, cb)
	return nil
}

func (c *Client) handleHistory(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.historyHandler == nil {
		return ErrorNotAvailable
	}

	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeHistory(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding history", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	channel := cmd.Channel
	if channel == "" {
		return DisconnectBadRequest
	}

	chOpts, found, err := c.node.channelOptions(channel)
	if err != nil {
		return err
	}
	if !found {
		return ErrorUnknownChannel
	}
	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		return ErrorNotAvailable
	}

	event := HistoryEvent{
		Channel: channel,
	}

	cb := func(reply HistoryReply, err error) {
		defer rw.done()
		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}

		historyResult, err := c.node.fullHistory(channel)
		if err != nil {
			c.node.logger.log(newLogEntry(errLogLevel(err), "error getting history", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			c.writeErrorFlush(rw, toClientErr(err))
			return
		}

		pubs := make([]*protocol.Publication, 0, len(historyResult.Publications))
		for _, pub := range historyResult.Publications {
			protoPub := pubToProto(pub)
			if hasFlag(CompatibilityFlags, UseSeqGen) {
				protoPub.Seq, protoPub.Gen = recovery.UnpackUint64(protoPub.Offset)
			}
			pubs = append(pubs, protoPub)
		}

		replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeHistoryResult(&protocol.HistoryResult{
			Publications: pubs,
		})
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence stats", map[string]interface{}{"error": err.Error()}))
			c.writeErrorFlush(rw, ErrorInternal)
			return
		}
		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	c.eventHub.historyHandler(event, cb)
	return nil
}

func (c *Client) handlePing(params protocol.Raw, rw *replyWriter) error {
	_, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePing(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding ping", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	_ = writeReplyFlush(rw, &protocol.Reply{})
	defer rw.done()
	return nil
}

func (c *Client) writeErrorFlush(rw *replyWriter, error *Error) {
	_ = rw.write(&protocol.Reply{Error: error.toProto()})
	_ = rw.flush()
}

func (c *Client) writeDisconnectOrErrorFlush(rw *replyWriter, replyError error) {
	switch t := replyError.(type) {
	case *Disconnect:
		go func() { _ = c.close(t) }()
		return
	default:
		c.writeErrorFlush(rw, toClientErr(replyError))
	}
}

func writeReplyFlush(rw *replyWriter, reply *protocol.Reply) error {
	err := rw.write(reply)
	if err != nil {
		return err
	}
	err = rw.flush()
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) handleRPC(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.rpcHandler == nil {
		return ErrorNotAvailable
	}
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeRPC(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding rpc", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}

	event := RPCEvent{
		Method: cmd.Method,
		Data:   cmd.Data,
	}

	cb := func(reply RPCReply, err error) {
		defer rw.done()
		if err != nil {
			c.writeDisconnectOrErrorFlush(rw, err)
			return
		}
		result := &protocol.RPCResult{
			Data: reply.Data,
		}
		var replyRes []byte
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeRPCResult(result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding rpc", map[string]interface{}{"error": err.Error()}))
			c.writeErrorFlush(rw, ErrorInternal)
			return
		}
		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	c.eventHub.rpcHandler(event, cb)
	return nil
}

func (c *Client) handleSend(params protocol.Raw, rw *replyWriter) error {
	if c.eventHub.messageHandler == nil {
		// send handler is a bit special since it is only one way
		// request: client does not expect any reply.
		rw.done()
		return nil
	}
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeSend(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding message", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	defer rw.done()
	c.eventHub.messageHandler(MessageEvent{
		Data: cmd.Data,
	})
	return nil
}

func (c *Client) unlockServerSideSubscriptions(subCtxMap map[string]subscribeContext) {
	for channel := range subCtxMap {
		c.pubSubSync.StopBuffering(channel)
	}
}

// connectCmd handles connect command from client - client must send connect
// command immediately after establishing connection with server.
func (c *Client) connectCmd(cmd *protocol.ConnectRequest, rw *replyWriter) error {
	c.mu.RLock()
	authenticated := c.authenticated
	closed := c.status == statusClosed
	c.mu.RUnlock()

	if closed {
		return DisconnectNormal
	}

	if authenticated {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already authenticated", map[string]interface{}{"client": c.uid, "user": c.user}))
		return DisconnectBadRequest
	}

	config := c.node.config
	version := config.Version
	userConnectionLimit := config.ClientUserConnectionLimit

	var (
		credentials       *Credentials
		authData          protocol.Raw
		channels          []string
		clientSideRefresh bool
	)

	if c.node.clientEvents.connectingHandler != nil {
		reply, err := c.node.clientEvents.connectingHandler(c.ctx, ConnectEvent{
			ClientID:  c.ID(),
			Data:      cmd.Data,
			Token:     cmd.Token,
			Name:      cmd.Name,
			Version:   cmd.Version,
			Transport: c.transport,
		})
		if err != nil {
			return err
		}
		if reply.Credentials != nil {
			credentials = reply.Credentials
		}
		if reply.Context != nil {
			c.mu.Lock()
			c.ctx = reply.Context
			c.mu.Unlock()
		}
		if reply.Data != nil {
			authData = reply.Data
		}
		clientSideRefresh = reply.ClientSideRefresh
		channels = append(channels, reply.Channels...)
	}

	if credentials == nil {
		// Try to find Credentials in context.
		if cred, ok := GetCredentials(c.ctx); ok {
			credentials = cred
		}
	}

	var (
		expires bool
		ttl     uint32
	)

	c.mu.Lock()
	c.clientSideRefresh = clientSideRefresh
	c.mu.Unlock()

	if credentials == nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client credentials not found", map[string]interface{}{"client": c.uid}))
		return DisconnectBadRequest
	}

	c.mu.Lock()
	c.user = credentials.UserID
	c.info = credentials.Info
	c.exp = credentials.ExpireAt

	user := c.user
	exp := c.exp
	closed = c.status == statusClosed
	c.mu.Unlock()

	if closed {
		return DisconnectNormal
	}

	c.node.logger.log(newLogEntry(LogLevelDebug, "client authenticated", map[string]interface{}{"client": c.uid, "user": c.user}))

	if userConnectionLimit > 0 && user != "" && len(c.node.hub.userConnections(user)) >= userConnectionLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "limit of connections for user reached", map[string]interface{}{"user": user, "client": c.uid, "limit": userConnectionLimit}))
		return ErrorLimitExceeded
	}

	c.mu.RLock()
	if exp > 0 {
		expires = true
		now := time.Now().Unix()
		if exp < now {
			c.mu.RUnlock()
			c.node.logger.log(newLogEntry(LogLevelInfo, "connection expiration must be greater than now", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
			return ErrorExpired
		}
		ttl = uint32(exp - now)
	}
	c.mu.RUnlock()

	res := &protocol.ConnectResult{
		Version: version,
		Expires: expires,
		TTL:     ttl,
	}

	// Client successfully connected.
	c.mu.Lock()
	c.authenticated = true
	c.mu.Unlock()

	err := c.node.addClient(c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding client", map[string]interface{}{"client": c.uid, "error": err.Error()}))
		return DisconnectServerError
	}

	if !clientSideRefresh {
		// Server will do refresh itself.
		res.Expires = false
		res.TTL = 0
	}

	res.Client = c.uid
	if authData != nil {
		res.Data = authData
	}

	var subCtxMap map[string]subscribeContext
	if len(channels) > 0 {
		var subMu sync.Mutex
		subCtxMap = make(map[string]subscribeContext, len(channels))
		subs := make(map[string]*protocol.SubscribeResult, len(channels))
		var subDisconnect *Disconnect
		var subError *Error
		var wg sync.WaitGroup

		wg.Add(len(channels))
		for _, channel := range channels {
			go func(channel string) {
				defer wg.Done()
				subCmd := &protocol.SubscribeRequest{
					Channel: channel,
				}
				if subReq, ok := cmd.Subs[channel]; ok {
					subCmd.Recover = subReq.Recover
					subCmd.Offset = subReq.Offset
					subCmd.Epoch = subReq.Epoch
				}

				var subCtx subscribeContext
				chOpts, validateErr, validateDisconnect := c.validateSubscribeRequest(subCmd)
				if subError != nil || subDisconnect != nil {
					subCtx = subscribeContext{
						err:        validateErr,
						disconnect: validateDisconnect,
					}
				} else {
					subCtx = c.subscribeCmd(subCmd, chOpts, SubscribeReply{}, rw, true)
				}
				subMu.Lock()
				subs[channel] = subCtx.result
				subCtxMap[channel] = subCtx
				if subCtx.disconnect != nil {
					subDisconnect = subCtx.disconnect
				}
				if subCtx.err != nil {
					subError = subCtx.err
				}
				subMu.Unlock()
			}(channel)
		}
		wg.Wait()

		if subDisconnect != nil || subError != nil {
			c.unlockServerSideSubscriptions(subCtxMap)
			if subDisconnect != nil {
				return subDisconnect
			}
			return subError
		}
		res.Subs = subs
	}

	replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeConnectResult(res)
	if err != nil {
		c.unlockServerSideSubscriptions(subCtxMap)
		c.node.logger.log(newLogEntry(LogLevelError, "error encoding connect", map[string]interface{}{"error": err.Error()}))
		return DisconnectServerError
	}
	_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	defer rw.done()

	c.mu.Lock()
	for channel, subCtx := range subCtxMap {
		c.channels[channel] = subCtx.channelContext
	}
	c.mu.Unlock()

	c.unlockServerSideSubscriptions(subCtxMap)

	if len(subCtxMap) > 0 {
		for channel, subCtx := range subCtxMap {
			go func(channel string, subCtx subscribeContext) {
				if subCtx.chOpts.JoinLeave && subCtx.clientInfo != nil {
					_ = c.node.publishJoin(channel, subCtx.clientInfo)
				}
			}(channel, subCtx)
		}
	}

	return nil
}

// Subscribe client to channel.
func (c *Client) Subscribe(channel string) error {
	subCmd := &protocol.SubscribeRequest{
		Channel: channel,
	}
	chOpts, validateErr, validateDisconnect := c.validateSubscribeRequest(subCmd)
	if validateErr != nil {
		return validateErr
	}
	if validateDisconnect != nil {
		return validateDisconnect
	}
	subCtx := c.subscribeCmd(subCmd, chOpts, SubscribeReply{}, nil, true)
	if subCtx.err != nil {
		return subCtx.err
	}
	defer c.pubSubSync.StopBuffering(channel)
	c.mu.Lock()
	c.channels[channel] = subCtx.channelContext
	c.mu.Unlock()
	pushEncoder := protocol.GetPushEncoder(c.transport.Protocol().toProto())
	sub := &protocol.Sub{
		Offset:      subCtx.result.GetOffset(),
		Epoch:       subCtx.result.GetEpoch(),
		Recoverable: subCtx.result.GetRecoverable(),
	}
	if hasFlag(CompatibilityFlags, UseSeqGen) {
		sub.Seq, sub.Gen = recovery.UnpackUint64(subCtx.result.GetOffset())
	}
	data, err := pushEncoder.EncodeSub(sub)
	if err != nil {
		return err
	}
	result, err := pushEncoder.Encode(clientproto.NewSubPush(channel, data))
	if err != nil {
		return err
	}
	reply := prepared.NewReply(&protocol.Reply{
		Result: result,
	}, c.transport.Protocol().toProto())
	return c.transportEnqueue(reply)
}

func (c *Client) validateSubscribeRequest(cmd *protocol.SubscribeRequest) (ChannelOptions, *Error, *Disconnect) {
	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for subscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return ChannelOptions{}, nil, DisconnectBadRequest
	}

	chOpts, found, err := c.node.channelOptions(channel)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error getting channel options", map[string]interface{}{"error": err.Error(), "channel": channel, "user": c.user, "client": c.uid}))
		return ChannelOptions{}, toClientErr(err), nil
	}
	if !found {
		c.node.logger.log(newLogEntry(LogLevelInfo, "subscription to unknown channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		return ChannelOptions{}, ErrorUnknownChannel, nil
	}

	config := c.node.config
	channelMaxLength := config.ChannelMaxLength
	channelLimit := config.ClientChannelLimit

	if channelMaxLength > 0 && len(channel) > channelMaxLength {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel too long", map[string]interface{}{"max": channelMaxLength, "channel": channel, "user": c.user, "client": c.uid}))
		return ChannelOptions{}, ErrorLimitExceeded, nil
	}

	c.mu.RLock()
	numChannels := len(c.channels)
	c.mu.RUnlock()

	if channelLimit > 0 && numChannels >= channelLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "maximum limit of channels per client reached", map[string]interface{}{"limit": channelLimit, "user": c.user, "client": c.uid}))
		return ChannelOptions{}, ErrorLimitExceeded, nil
	}

	c.mu.RLock()
	_, ok := c.channels[channel]
	c.mu.RUnlock()
	if ok {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already subscribed on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		return ChannelOptions{}, ErrorAlreadySubscribed, nil
	}

	return chOpts, nil, nil
}

func errorDisconnectContext(replyError *Error, disconnect *Disconnect) subscribeContext {
	ctx := subscribeContext{}
	if disconnect != nil {
		ctx.disconnect = disconnect
		return ctx
	}
	if replyError != nil {
		ctx.err = replyError
		return ctx
	}
	return ctx
}

type subscribeContext struct {
	result         *protocol.SubscribeResult
	chOpts         ChannelOptions
	clientInfo     *ClientInfo
	err            *Error
	disconnect     *Disconnect
	channelContext channelContext
}

func isRecovered(historyResult HistoryResult, cmdOffset uint64, cmdEpoch string) ([]*protocol.Publication, bool) {
	latestOffset := historyResult.Offset
	latestEpoch := historyResult.Epoch

	recoveredPubs := make([]*protocol.Publication, 0, len(historyResult.Publications))
	for _, pub := range historyResult.Publications {
		protoPub := pubToProto(pub)
		recoveredPubs = append(recoveredPubs, protoPub)
	}

	nextOffset := cmdOffset + 1
	var recovered bool
	if len(recoveredPubs) == 0 {
		recovered = latestOffset == cmdOffset && latestEpoch == cmdEpoch
	} else {
		recovered = recoveredPubs[0].Offset == nextOffset && latestEpoch == cmdEpoch
	}

	return recoveredPubs, recovered
}

// subscribeCmd handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel. Optionally we can send missed messages to
// client if it provided last message id seen in channel.
func (c *Client) subscribeCmd(cmd *protocol.SubscribeRequest, chOpts ChannelOptions, reply SubscribeReply, rw *replyWriter, serverSide bool) subscribeContext {

	ctx := subscribeContext{}
	res := &protocol.SubscribeResult{}

	if reply.ExpireAt > 0 {
		ttl := reply.ExpireAt - time.Now().Unix()
		if ttl <= 0 {
			c.node.logger.log(newLogEntry(LogLevelInfo, "subscription expiration must be greater than now", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
			return errorDisconnectContext(ErrorExpired, nil)
		}
		if reply.ClientSideRefresh {
			res.Expires = true
			res.TTL = uint32(ttl)
		}
	}

	channel := cmd.Channel

	info := &ClientInfo{
		ClientID: c.uid,
		UserID:   c.user,
		ConnInfo: c.info,
		ChanInfo: reply.ChannelInfo,
	}

	if chOpts.HistoryRecover {
		c.publicationsOnce.Do(func() {
			// We need extra processing for Publications in channels with recovery feature on.
			// This extra processing routine help us to do all necessary actions without blocking
			// broadcasts inside Hub for a long time.
			go c.processPublications()
		})
		// Start syncing recovery and PUB/SUB.
		// The important thing is to call StopBuffering for this channel
		// after response with Publications written to connection.
		c.pubSubSync.StartBuffering(channel)
	}

	err := c.node.addSubscription(channel, c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		c.pubSubSync.StopBuffering(channel)
		if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
			return errorDisconnectContext(clientErr, nil)
		}
		ctx.disconnect = DisconnectServerError
		return ctx
	}

	if chOpts.Presence {
		err = c.node.addPresence(channel, c.uid, info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error adding presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			c.pubSubSync.StopBuffering(channel)
			ctx.disconnect = DisconnectServerError
			return ctx
		}
	}

	var (
		latestOffset  uint64
		latestEpoch   string
		recoveredPubs []*protocol.Publication
	)

	useSeqGen := hasFlag(CompatibilityFlags, UseSeqGen)

	if chOpts.HistoryRecover {
		res.Recoverable = true
		if cmd.Recover {
			cmdOffset := cmd.Offset
			if cmd.Seq > 0 || cmd.Gen > 0 {
				// Fallback to deprecated fields.
				cmdOffset = recovery.PackUint64(cmd.Seq, cmd.Gen)
			}

			// Client provided subscribe request with recover flag on. Try to recover missed
			// publications automatically from history (we suppose here that history configured wisely).
			historyResult, err := c.node.recoverHistory(channel, StreamPosition{cmdOffset, cmd.Epoch})
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error on recover", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				c.pubSubSync.StopBuffering(channel)
				if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
					return errorDisconnectContext(clientErr, nil)
				}
				ctx.disconnect = DisconnectServerError
				return ctx
			}
			latestOffset = historyResult.Offset
			latestEpoch = historyResult.Epoch
			var recovered bool
			recoveredPubs, recovered = isRecovered(historyResult, cmdOffset, cmd.Epoch)
			res.Recovered = recovered
			incRecover(res.Recovered)
		} else {
			streamTop, err := c.node.streamTop(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error getting recovery state for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				c.pubSubSync.StopBuffering(channel)
				if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
					return errorDisconnectContext(clientErr, nil)
				}
				ctx.disconnect = DisconnectServerError
				return ctx
			}
			latestOffset = streamTop.Offset
			latestEpoch = streamTop.Epoch
		}

		res.Epoch = latestEpoch

		if useSeqGen {
			res.Seq, res.Gen = recovery.UnpackUint64(latestOffset)
		} else {
			res.Offset = latestOffset
		}

		c.pubSubSync.LockBuffer(channel)
		bufferedPubs := c.pubSubSync.ReadBuffered(channel)
		var okMerge bool
		recoveredPubs, okMerge = recovery.MergePublications(recoveredPubs, bufferedPubs, useSeqGen)
		if !okMerge {
			c.pubSubSync.StopBuffering(channel)
			ctx.disconnect = DisconnectServerError
			return ctx
		}
	}

	if len(recoveredPubs) > 0 {
		if useSeqGen {
			// recoveredPubs are in descending order.
			latestOffset = recoveredPubs[0].Offset
		} else {
			latestOffset = recoveredPubs[len(recoveredPubs)-1].Offset
		}
	}

	res.Publications = recoveredPubs
	if useSeqGen && len(res.Publications) > 0 {
		for i := range res.Publications {
			res.Publications[i].Seq, res.Publications[i].Gen = recovery.UnpackUint64(res.Publications[i].Offset)
			res.Publications[i].Offset = 0
		}
	}

	if !serverSide {
		// Write subscription reply only if initiated by client.
		replyRes, err := protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeSubscribeResult(res)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding subscribe", map[string]interface{}{"error": err.Error()}))
			if !serverSide {
				// Will be called later in case of server side sub.
				c.pubSubSync.StopBuffering(channel)
			}
			ctx.disconnect = DisconnectServerError
			return ctx
		}
		// Need to flush data from writer so subscription response is
		// sent before any subscription publication.
		_ = writeReplyFlush(rw, &protocol.Reply{Result: replyRes})
	}

	channelContext := channelContext{
		Info:              reply.ChannelInfo,
		serverSide:        serverSide,
		clientSideRefresh: reply.ClientSideRefresh,
		expireAt:          reply.ExpireAt,
		streamPosition: StreamPosition{
			Offset: latestOffset,
			Epoch:  latestEpoch,
		},
	}
	if chOpts.HistoryRecover {
		channelContext.positionCheckTime = time.Now().Unix()
	}

	if !serverSide {
		c.mu.Lock()
		c.channels[channel] = channelContext
		c.mu.Unlock()
		// Stop syncing recovery and PUB/SUB.
		// In case of server side subscription we will do this later.
		c.pubSubSync.StopBuffering(channel)
	}

	if c.node.logger.enabled(LogLevelDebug) {
		c.node.logger.log(newLogEntry(LogLevelDebug, "client subscribed to channel", map[string]interface{}{"client": c.uid, "user": c.user, "channel": cmd.Channel}))
	}

	ctx.result = res
	ctx.chOpts = chOpts
	ctx.clientInfo = info
	ctx.channelContext = channelContext
	return ctx
}

func (c *Client) writePublicationUpdatePosition(ch string, pub *protocol.Publication, reply *prepared.Reply, chOpts *ChannelOptions) error {
	if !chOpts.HistoryRecover {
		return c.transportEnqueue(reply)
	}
	c.mu.Lock()
	channelContext, ok := c.channels[ch]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	currentPositionOffset := channelContext.streamPosition.Offset
	nextExpectedOffset := currentPositionOffset + 1
	pubOffset := pub.Offset
	if pubOffset != nextExpectedOffset {
		if c.node.logger.enabled(LogLevelDebug) {
			c.node.logger.log(newLogEntry(LogLevelDebug, "client insufficient state", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "offset": pubOffset, "expectedOffset": nextExpectedOffset}))
		}
		// Oops: sth lost, let client reconnect to recover its state.
		go func() { _ = c.close(DisconnectInsufficientState) }()
		c.mu.Unlock()
		return nil
	}
	channelContext.positionCheckTime = time.Now().Unix()
	channelContext.positionCheckFailures = 0
	channelContext.streamPosition.Offset = pub.Offset
	c.channels[ch] = channelContext
	c.mu.Unlock()
	return c.transportEnqueue(reply)
}

// 10Mb, should not be achieved in normal conditions, if the limit
// is reached then this is a signal to scale.
const maxPubQueueSize = 10485760

func (c *Client) writePublication(ch string, pub *protocol.Publication, reply *prepared.Reply, chOpts *ChannelOptions) error {
	if !chOpts.HistoryRecover {
		return c.transportEnqueue(reply)
	}
	c.publicationsOnce.Do(func() {
		// We need extra processing for Publications in channels with recovery feature on.
		// This extra processing routine help us to do all necessary actions without blocking
		// broadcasts inside Hub for a long time.
		go c.processPublications()
	})
	if ok := c.publications.Add(preparedPub{
		reply:   reply,
		channel: ch,
		pub:     pub,
		chOpts:  chOpts,
	}); !ok {
		return nil
	}
	if c.publications.Size() > maxPubQueueSize {
		go func() { _ = c.close(DisconnectServerError) }()
	}
	return nil
}

func (c *Client) processPublications() {
	for {
		// Wait for pub from queue.
		p, ok := c.publications.Wait()
		if !ok {
			if c.publications.Closed() {
				return
			}
			continue
		}
		c.pubSubSync.SyncPublication(p.channel, p.pub, func() {
			_ = c.writePublicationUpdatePosition(p.channel, p.pub, p.reply, p.chOpts)
		})
	}
}

func (c *Client) writeJoin(_ string, reply *prepared.Reply) error {
	return c.transportEnqueue(reply)
}

func (c *Client) writeLeave(_ string, reply *prepared.Reply) error {
	return c.transportEnqueue(reply)
}

// Lock must be held outside.
func (c *Client) unsubscribe(channel string) error {
	chOpts, _, err := c.node.channelOptions(channel)
	if err != nil {
		return err
	}

	c.mu.RLock()
	info := c.clientInfo(channel)
	chCtx, ok := c.channels[channel]
	serverSide := chCtx.serverSide
	c.mu.RUnlock()

	if ok {
		c.mu.Lock()
		delete(c.channels, channel)
		c.mu.Unlock()

		if chOpts.Presence {
			err := c.node.removePresence(channel, c.uid)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error removing channel presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			}
		}

		if chOpts.JoinLeave {
			_ = c.node.publishLeave(channel, info)
		}

		if err := c.node.removeSubscription(channel, c); err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error removing subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			return err
		}

		if !serverSide && c.eventHub.unsubscribeHandler != nil {
			c.eventHub.unsubscribeHandler(UnsubscribeEvent{
				Channel: channel,
			})
		}
	}
	if c.node.logger.enabled(LogLevelDebug) {
		c.node.logger.log(newLogEntry(LogLevelDebug, "client unsubscribed from channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
	}
	return nil
}

func toClientErr(err error) *Error {
	if clientErr, ok := err.(*Error); ok {
		return clientErr
	}
	return ErrorInternal
}

func errLogLevel(err error) LogLevel {
	logLevel := LogLevelInfo
	if err != ErrorNotAvailable {
		logLevel = LogLevelError
	}
	return logLevel
}
