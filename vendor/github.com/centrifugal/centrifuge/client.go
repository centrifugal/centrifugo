package centrifuge

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/uuid"

	"github.com/dgrijalva/jwt-go"
)

// ClientEventHub allows to deal with client event handlers.
// All its methods are not goroutine-safe and supposed to be called once on client connect.
type ClientEventHub struct {
	disconnectHandler  DisconnectHandler
	subscribeHandler   SubscribeHandler
	unsubscribeHandler UnsubscribeHandler
	publishHandler     PublishHandler
	subRefreshHandler  SubRefreshHandler
	rpcHandler         RPCHandler
	messageHandler     MessageHandler
}

// Disconnect allows to set DisconnectHandler.
// DisconnectHandler called when client disconnected.
func (c *ClientEventHub) Disconnect(h DisconnectHandler) {
	c.disconnectHandler = h
}

// Message allows to set MessageHandler.
// MessageHandler called when client sent asynchronous message.
func (c *ClientEventHub) Message(h MessageHandler) {
	c.messageHandler = h
}

// RPC allows to set RPCHandler.
// RPCHandler will be executed on every incoming RPC call.
func (c *ClientEventHub) RPC(h RPCHandler) {
	c.rpcHandler = h
}

// SubRefresh allows to set SubRefreshHandler.
// SubRefreshHandler called when it's time to refresh client subscription.
func (c *ClientEventHub) SubRefresh(h SubRefreshHandler) {
	c.subRefreshHandler = h
}

// Subscribe allows to set SubscribeHandler.
// SubscribeHandler called when client subscribes on channel.
func (c *ClientEventHub) Subscribe(h SubscribeHandler) {
	c.subscribeHandler = h
}

// Unsubscribe allows to set UnsubscribeHandler.
// UnsubscribeHandler called when client unsubscribes from channel.
func (c *ClientEventHub) Unsubscribe(h UnsubscribeHandler) {
	c.unsubscribeHandler = h
}

// Publish allows to set PublishHandler.
// PublishHandler called when client publishes message into channel.
func (c *ClientEventHub) Publish(h PublishHandler) {
	c.publishHandler = h
}

// We poll current position check from history storage periodically.
// If client position is wrong maxCheckPositionFailures times in a row
// then client will be disconnected.
const maxCheckPositionFailures int64 = 2

// ChannelContext contains extra context for channel connection subscribed to.
type ChannelContext struct {
	Info                  proto.Raw
	expireAt              int64
	positionCheckTime     int64
	positionCheckFailures int64
	recoveryPosition      RecoveryPosition
}

// Client represents client connection to server.
type Client struct {
	mu sync.RWMutex

	// presenceMu allows to sync presence routine with client closing.
	presenceMu sync.Mutex

	ctx       context.Context
	node      *Node
	transport Transport

	closed        bool
	authenticated bool

	uid  string
	user string
	exp  int64
	info proto.Raw

	channels map[string]ChannelContext

	staleTimer    *time.Timer
	expireTimer   *time.Timer
	presenceTimer *time.Timer

	disconnect *Disconnect

	eventHub *ClientEventHub

	// The following fields help us to synchronize PUB/SUB and history messages during
	// publication recovery process. At moment we use the fact that subscription requests
	// processed by client in sequence so only keep a reference to one channel that is
	// in subscribe process. If we want to process subscriptions in parallel in future
	// then we have to use separate flags and buffers for each channel here - i.e. maybe
	// use map.
	inSubscribe     uint32
	inSubscribeChMu sync.RWMutex
	inSubscribeCh   string
	pubBufferMu     sync.Mutex
	pubBuffer       []*Publication

	messageWriter *writer
}

// NewClient initializes new Client.
func NewClient(ctx context.Context, n *Node, t Transport) (*Client, error) {

	uuidObject, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}

	config := n.Config()

	c := &Client{
		ctx:       ctx,
		uid:       uuidObject.String(),
		node:      n,
		transport: t,
		eventHub:  &ClientEventHub{},
		pubBuffer: make([]*Publication, 0),
		channels:  make(map[string]ChannelContext),
	}

	transportMessagesSentCounter := transportMessagesSent.WithLabelValues(t.Name())

	messageWriterConf := writerConfig{
		MaxQueueSize: config.ClientQueueMaxSize,
		WriteFn: func(data []byte) error {
			err := t.Write(data)
			if err != nil {
				go c.Close(DisconnectWriteError)
				return err
			}
			transportMessagesSentCounter.Inc()
			return nil
		},
		WriteManyFn: func(data ...[]byte) error {
			buf := getBuffer()
			for _, payload := range data {
				buf.Write(payload)
			}
			err := t.Write(buf.Bytes())
			if err != nil {
				go c.Close(DisconnectWriteError)
				putBuffer(buf)
				return err
			}
			putBuffer(buf)
			transportMessagesSentCounter.Add(float64(len(data)))
			return nil
		},
	}

	c.messageWriter = newWriter(messageWriterConf)

	staleCloseDelay := config.ClientStaleCloseDelay
	if staleCloseDelay > 0 && !c.authenticated {
		c.mu.Lock()
		c.staleTimer = time.AfterFunc(staleCloseDelay, c.closeUnauthenticated)
		c.mu.Unlock()
	}

	return c, nil
}

// closeUnauthenticated closes connection if it's not authenticated yet.
// At moment used to close client connections which have not sent valid
// connect command in a reasonable time interval after established connection
// with server.
func (c *Client) closeUnauthenticated() {
	c.mu.RLock()
	authenticated := c.authenticated
	closed := c.closed
	c.mu.RUnlock()
	if !authenticated && !closed {
		c.Close(DisconnectStale)
	}
}

func (c *Client) transportSend(reply *preparedReply) error {
	data := reply.Data()
	disconnect := c.messageWriter.enqueue(data)
	if disconnect != nil {
		// Close in goroutine to not block message broadcast.
		go c.Close(disconnect)
		return io.EOF
	}
	return nil
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect.
func (c *Client) updateChannelPresence(ch string) error {
	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		return nil
	}
	if !chOpts.Presence {
		return nil
	}
	info := c.clientInfo(ch)
	return c.node.addPresence(ch, c.uid, info)
}

func (c *Client) checkSubscriptionExpiration(channel string, channelContext ChannelContext, delay time.Duration) bool {
	now := time.Now().Unix()
	expireAt := channelContext.expireAt
	if expireAt > 0 && now > expireAt+int64(delay.Seconds()) {
		// Subscription expired.
		if c.eventHub.subRefreshHandler != nil {
			// Give subscription a chance to be refreshed via SubRefreshHandler.
			reply := c.eventHub.subRefreshHandler(SubRefreshEvent{Channel: channel})
			if reply.Expired || (reply.ExpireAt > 0 && reply.ExpireAt < now) {
				return false
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
		} else {
			// The only way subscription could be refreshed in this case is via
			// SUB_REFRESH command sent from client but looks like that command
			// with new refreshed token have not been received in configured window.
			return false
		}
	}
	return true
}

// updatePresence used for various periodic actions we need to do with client connections.
func (c *Client) updatePresence() {
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	config := c.node.Config()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	channels := make(map[string]ChannelContext, len(c.channels))
	for channel, channelContext := range c.channels {
		channels[channel] = channelContext
	}
	c.mu.Unlock()
	for channel, channelContext := range channels {
		if !c.checkSubscriptionExpiration(channel, channelContext, config.ClientExpiredSubCloseDelay) {
			// Ideally we should deal with single expired subscription in this
			// case - i.e. unsubscribe client from channel and give an advice
			// to resubscribe. But there is scenario when browser goes online
			// after computer was in sleeping mode which I have not managed to
			// handle reliably on client side when unsubscribe with resubscribe
			// flag was used. So I decided to stick with disconnect for now -
			// it seems to work fine and drastically simplifies client code.
			go c.Close(DisconnectSubExpired)
			// No need to proceed after close.
			return
		}

		checkDelay := config.ClientChannelPositionCheckDelay
		if checkDelay > 0 && !c.checkPosition(checkDelay, channel, channelContext) {
			go c.Close(DisconnectInsufficientState)
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

// Lock must be held outside.
func (c *Client) addPresenceUpdate() {
	config := c.node.Config()
	presenceInterval := config.ClientPresencePingInterval
	c.presenceTimer = time.AfterFunc(presenceInterval, c.updatePresence)
}

// Lock must be held outside.
func (c *Client) checkPosition(checkDelay time.Duration, ch string, channelContext ChannelContext) bool {
	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		return true
	}
	if !chOpts.HistoryRecover {
		return true
	}
	nowUnix := time.Now().Unix()
	needCheckPosition := channelContext.positionCheckTime == 0 || nowUnix-channelContext.positionCheckTime > int64(checkDelay.Seconds())
	if !needCheckPosition {
		return true
	}
	position := channelContext.recoveryPosition
	streamPosition, err := c.node.currentRecoveryState(ch)
	if err != nil {
		return true
	}

	isValidPosition := streamPosition.Seq == position.Seq && streamPosition.Gen == position.Gen && streamPosition.Epoch == position.Epoch
	keepConnection := true
	c.mu.Lock()
	if channelContext, ok = c.channels[ch]; ok {
		channelContext.positionCheckTime = nowUnix
		if !isValidPosition {
			channelContext.positionCheckFailures++
			keepConnection = channelContext.positionCheckFailures < maxCheckPositionFailures
		} else {
			channelContext.positionCheckFailures = 0
		}
		c.channels[ch] = channelContext
	}
	c.mu.Unlock()
	return keepConnection
}

// ID returns unique client connection id.
func (c *Client) ID() string {
	return c.uid
}

// UserID returns user ID associated with client connection.
func (c *Client) UserID() string {
	return c.user
}

// Transport returns transport details used by client connection.
func (c *Client) Transport() TransportInfo {
	return c.transport
}

// Channels returns a map of channels client connection currently subscribed to.
func (c *Client) Channels() map[string]ChannelContext {
	c.mu.RLock()
	defer c.mu.RUnlock()
	channels := make(map[string]ChannelContext, len(c.channels))
	for ch, ctx := range c.channels {
		channels[ch] = ctx
	}
	return channels
}

// On returns ClientEventHub to set various event handlers to client.
func (c *Client) On() *ClientEventHub {
	return c.eventHub
}

// Send data to client connection asynchronously.
func (c *Client) Send(data Raw) error {
	p := &proto.Message{
		Data: data,
	}

	pushEncoder := proto.GetPushEncoder(c.transport.Protocol())
	data, err := pushEncoder.EncodeMessage(p)
	if err != nil {
		return err
	}
	result, err := pushEncoder.Encode(proto.NewMessagePush(data))
	if err != nil {
		return err
	}

	reply := newPreparedReply(&proto.Reply{
		Result: result,
	}, c.transport.Protocol())

	return c.transportSend(reply)
}

// Unsubscribe allows to unsubscribe client from channel.
func (c *Client) Unsubscribe(ch string, resubscribe bool) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	err := c.unsubscribe(ch)
	if err != nil {
		return err
	}
	return c.sendUnsub(ch, resubscribe)
}

func (c *Client) sendUnsub(ch string, resubscribe bool) error {
	pushEncoder := proto.GetPushEncoder(c.transport.Protocol())

	data, err := pushEncoder.EncodeUnsub(&proto.Unsub{Resubscribe: resubscribe})
	if err != nil {
		return err
	}
	result, err := pushEncoder.Encode(proto.NewUnsubPush(ch, data))
	if err != nil {
		return err
	}

	reply := newPreparedReply(&proto.Reply{
		Result: result,
	}, c.transport.Protocol())

	c.transportSend(reply)

	return nil
}

// Close client connection with specific disconnect reason.
func (c *Client) Close(disconnect *Disconnect) error {
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true

	if c.disconnect != nil {
		disconnect = c.disconnect
	}

	channels := make(map[string]ChannelContext, len(c.channels))
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

	c.mu.Lock()
	if c.expireTimer != nil {
		c.expireTimer.Stop()
	}
	if c.presenceTimer != nil {
		c.presenceTimer.Stop()
	}
	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}
	c.mu.Unlock()

	// Close writer and send messages remaining in writer queue if any.
	c.messageWriter.close()

	c.transport.Close(disconnect)

	if disconnect != nil && disconnect.Reason != "" {
		c.node.logger.log(newLogEntry(LogLevelDebug, "closing client connection", map[string]interface{}{"client": c.uid, "user": c.user, "reason": disconnect.Reason, "reconnect": disconnect.Reconnect}))
	}
	if disconnect != nil {
		serverDisconnectCount.WithLabelValues(strconv.Itoa(disconnect.Code)).Inc()
	}
	if c.eventHub.disconnectHandler != nil {
		c.eventHub.disconnectHandler(DisconnectEvent{
			Disconnect: disconnect,
		})
	}

	return nil
}

func (c *Client) isInSubscribe(ch string) bool {
	if atomic.LoadUint32(&c.inSubscribe) == 1 {
		c.inSubscribeChMu.RLock()
		channelMatch := c.inSubscribeCh == ch
		c.inSubscribeChMu.RUnlock()
		return channelMatch
	}
	return false
}

func (c *Client) setInSubscribe(ch string, flag bool) {
	c.inSubscribeChMu.Lock()
	if flag {
		atomic.StoreUint32(&c.inSubscribe, 1)
		c.inSubscribeCh = ch
	} else {
		atomic.StoreUint32(&c.inSubscribe, 0)
		c.inSubscribeCh = ""
	}
	c.inSubscribeChMu.Unlock()
}

// Lock must be held outside.
func (c *Client) clientInfo(ch string) *proto.ClientInfo {
	var channelInfo proto.Raw
	channelContext, ok := c.channels[ch]
	if ok {
		channelInfo = channelContext.Info
	}
	return &proto.ClientInfo{
		User:     c.user,
		Client:   c.uid,
		ConnInfo: c.info,
		ChanInfo: channelInfo,
	}
}

// Handle raw data encoded with Centrifuge protocol. Not goroutine-safe.
func (c *Client) Handle(data []byte) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()

	if len(data) == 0 {
		c.node.logger.log(newLogEntry(LogLevelError, "empty client request received", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
		c.Close(DisconnectBadRequest)
		return false
	}

	protoType := c.transport.Protocol()

	encoder := proto.GetReplyEncoder(protoType)
	decoder := proto.GetCommandDecoder(protoType, data)

	for {
		cmd, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding command", map[string]interface{}{"data": string(data), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
			c.Close(DisconnectBadRequest)
			proto.PutCommandDecoder(protoType, decoder)
			proto.PutReplyEncoder(protoType, encoder)
			return false
		}
		var encodeErr error
		write := func(rep *proto.Reply) error {
			encodeErr = encoder.Encode(rep)
			if encodeErr != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error encoding reply", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "error": encodeErr.Error()}))
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
					proto.PutCommandDecoder(protoType, decoder)
					proto.PutReplyEncoder(protoType, encoder)
					c.Close(disconnect)
					return fmt.Errorf("flush error")
				}
			}
			encoder.Reset()
			return nil
		}
		disconnect := c.handleCommand(cmd, write, flush)
		if disconnect != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "disconnect after handling command", map[string]interface{}{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			c.Close(disconnect)
			proto.PutCommandDecoder(protoType, decoder)
			proto.PutReplyEncoder(protoType, encoder)
			return false
		}
		if encodeErr != nil {
			c.Close(DisconnectServerError)
			return false
		}
	}

	buf := encoder.Finish()
	if len(buf) > 0 {
		disconnect := c.messageWriter.enqueue(buf)
		if disconnect != nil {
			if c.node.logger.enabled(LogLevelDebug) {
				c.node.logger.log(newLogEntry(LogLevelDebug, "disconnect after sending reply", map[string]interface{}{"client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			}
			c.Close(disconnect)
			proto.PutCommandDecoder(protoType, decoder)
			proto.PutReplyEncoder(protoType, encoder)
			return false
		}
	}

	proto.PutCommandDecoder(protoType, decoder)
	proto.PutReplyEncoder(protoType, encoder)

	return true
}

type replyWriter struct {
	write func(*proto.Reply) error
	flush func() error
}

// handle dispatches Command into correct command handler.
func (c *Client) handleCommand(cmd *proto.Command, writeFn func(*proto.Reply) error, flush func() error) *Disconnect {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	var disconnect *Disconnect

	method := cmd.Method
	params := cmd.Params

	write := func(rep *proto.Reply) error {
		rep.ID = cmd.ID
		if rep.Error != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "client command error", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "error": rep.Error.Error()}))
			replyErrorCount.WithLabelValues(strings.ToLower(proto.MethodType_name[int32(method)]), strconv.FormatUint(uint64(rep.Error.Code), 10)).Inc()
		}
		return writeFn(rep)
	}

	rw := &replyWriter{write, flush}

	if cmd.ID == 0 && method != proto.MethodTypeSend {
		c.node.logger.log(newLogEntry(LogLevelInfo, "command ID required for commands with reply expected", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
		rw.write(&proto.Reply{Error: ErrorBadRequest})
		return nil
	}

	if method != proto.MethodTypeConnect && !c.authenticated {
		// Client must send connect command first.
		rw.write(&proto.Reply{Error: ErrorUnauthorized})
		return nil
	}

	started := time.Now()
	switch method {
	case proto.MethodTypeConnect:
		disconnect = c.handleConnect(params, rw)
	case proto.MethodTypeRefresh:
		disconnect = c.handleRefresh(params, rw)
	case proto.MethodTypeSubscribe:
		disconnect = c.handleSubscribe(params, rw)
	case proto.MethodTypeSubRefresh:
		disconnect = c.handleSubRefresh(params, rw)
	case proto.MethodTypeUnsubscribe:
		disconnect = c.handleUnsubscribe(params, rw)
	case proto.MethodTypePublish:
		disconnect = c.handlePublish(params, rw)
	case proto.MethodTypePresence:
		disconnect = c.handlePresence(params, rw)
	case proto.MethodTypePresenceStats:
		disconnect = c.handlePresenceStats(params, rw)
	case proto.MethodTypeHistory:
		disconnect = c.handleHistory(params, rw)
	case proto.MethodTypePing:
		disconnect = c.handlePing(params, rw)
	case proto.MethodTypeRPC:
		disconnect = c.handleRPC(params, rw)
	case proto.MethodTypeSend:
		disconnect = c.handleSend(params, rw)
	default:
		rw.write(&proto.Reply{Error: ErrorMethodNotFound})
	}
	commandDurationSummary.WithLabelValues(strings.ToLower(proto.MethodType_name[int32(method)])).Observe(time.Since(started).Seconds())
	return disconnect
}

func (c *Client) expire() {

	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return
	}

	now := time.Now().Unix()

	if c.node.eventHub.refreshHandler != nil {
		reply := c.node.eventHub.refreshHandler(c.ctx, c, RefreshEvent{})
		if reply.Expired {
			c.Close(DisconnectExpired)
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
	}

	c.mu.RLock()
	exp := c.exp
	c.mu.RUnlock()

	if exp == 0 {
		return
	}

	ttl := exp - now

	if c.node.eventHub.refreshHandler != nil {
		if ttl > 0 {
			c.mu.RLock()
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			c.mu.RUnlock()

			duration := time.Duration(ttl) * time.Second

			c.mu.Lock()
			if !c.closed {
				c.expireTimer = time.AfterFunc(duration, c.expire)
			}
			c.mu.Unlock()
		}
	}

	if ttl > 0 {
		// Connection was successfully refreshed.
		return
	}

	c.Close(DisconnectExpired)
}

func (c *Client) handleConnect(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeConnect(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding connect", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.connectCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodeConnectResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding connect", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}

	rw.write(&proto.Reply{Result: replyRes})
	rw.flush()
	if c.node.eventHub.connectedHandler != nil {
		c.node.eventHub.connectedHandler(c.ctx, c)
	}

	return nil
}

func (c *Client) handleRefresh(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding refresh", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.refreshCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodeRefreshResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding refresh", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleSubscribe(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeSubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding subscribe", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	return c.subscribeCmd(cmd, rw)
}

func (c *Client) handleSubRefresh(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeSubRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding sub refresh", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.subRefreshCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodeSubRefreshResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding sub refresh", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleUnsubscribe(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeUnsubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding unsubscribe", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.unsubscribeCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodeUnsubscribeResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding unsubscribe", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePublish(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodePublish(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding publish", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.publishCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodePublishResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding publish", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePresence(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodePresence(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.presenceCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodePresenceResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePresenceStats(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodePresenceStats(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence stats", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.presenceStatsCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodePresenceStatsResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence stats", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleHistory(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeHistory(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding history", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.historyCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodeHistoryResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding history", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePing(params proto.Raw, rw *replyWriter) *Disconnect {
	cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodePing(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding ping", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.pingCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		rw.write(&proto.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodePingResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding ping", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	rw.write(&proto.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleRPC(params proto.Raw, rw *replyWriter) *Disconnect {
	if c.eventHub.rpcHandler != nil {
		cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeRPC(params)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding rpc", map[string]interface{}{"error": err.Error()}))
			return DisconnectBadRequest
		}
		rpcReply := c.eventHub.rpcHandler(RPCEvent{
			Data: cmd.Data,
		})
		if rpcReply.Disconnect != nil {
			return rpcReply.Disconnect
		}
		if rpcReply.Error != nil {
			rw.write(&proto.Reply{Error: rpcReply.Error})
			return nil
		}

		result := &proto.RPCResult{
			Data: rpcReply.Data,
		}

		var replyRes []byte
		replyRes, err = proto.GetResultEncoder(c.transport.Protocol()).EncodeRPCResult(result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding rpc", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
		rw.write(&proto.Reply{Result: replyRes})
		return nil
	}
	rw.write(&proto.Reply{Error: ErrorNotAvailable})
	return nil
}

func (c *Client) handleSend(params proto.Raw, rw *replyWriter) *Disconnect {
	if c.eventHub.messageHandler != nil {
		cmd, err := proto.GetParamsDecoder(c.transport.Protocol()).DecodeSend(params)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding message", map[string]interface{}{"error": err.Error()}))
			return DisconnectBadRequest
		}
		messageReply := c.eventHub.messageHandler(MessageEvent{
			Data: cmd.Data,
		})
		if messageReply.Disconnect != nil {
			return messageReply.Disconnect
		}
		return nil
	}
	return nil
}

type connectTokenClaims struct {
	Info       proto.Raw `json:"info"`
	Base64Info string    `json:"b64info"`
	jwt.StandardClaims
}

type subscribeTokenClaims struct {
	Client     string    `json:"client"`
	Channel    string    `json:"channel"`
	Info       proto.Raw `json:"info"`
	Base64Info string    `json:"b64info"`
	jwt.StandardClaims
}

// connectCmd handles connect command from client - client must send connect
// command immediately after establishing connection with server.
func (c *Client) connectCmd(cmd *proto.ConnectRequest) (*proto.ConnectResponse, *Disconnect) {
	resp := &proto.ConnectResponse{}

	c.mu.RLock()
	authenticated := c.authenticated
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return resp, DisconnectNormal
	}

	if authenticated {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already authenticated", map[string]interface{}{"client": c.uid, "user": c.user}))
		return resp, DisconnectBadRequest
	}

	config := c.node.Config()
	version := config.Version
	insecure := config.ClientInsecure
	clientAnonymous := config.ClientAnonymous
	closeDelay := config.ClientExpiredCloseDelay
	userConnectionLimit := config.ClientUserConnectionLimit

	var credentials *Credentials
	var authData proto.Raw

	if c.node.eventHub.connectingHandler != nil {
		reply := c.node.eventHub.connectingHandler(c.ctx, c.transport, ConnectEvent{
			ClientID: c.ID(),
			Data:     cmd.Data,
			Token:    cmd.Token,
		})
		if reply.Disconnect != nil {
			return nil, reply.Disconnect
		}
		if reply.Error != nil {
			resp.Error = reply.Error
			return resp, nil
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
	}

	if credentials == nil {
		// Try to find Credentials in context.
		if val := c.ctx.Value(credentialsContextKey); val != nil {
			if creds, ok := val.(*Credentials); ok {
				credentials = creds
			}
		}
	}

	var expires bool
	var ttl uint32

	if credentials != nil {
		// Server-side auth.
		c.mu.Lock()
		c.user = credentials.UserID
		c.info = credentials.Info
		c.exp = credentials.ExpireAt
		c.mu.Unlock()
	} else if cmd.Token != "" {
		// Explicit auth Credentials not provided in auth handler and in context, try
		// to extract credentials from connection JWT.
		// TODO: JWT stuff should be outside library.
		token := cmd.Token

		var user string
		var info proto.Raw
		var b64info string
		var exp int64

		parsedToken, err := jwt.ParseWithClaims(token, &connectTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			if config.Secret == "" {
				return nil, fmt.Errorf("secret not set")
			}
			return []byte(config.Secret), nil
		})
		if parsedToken == nil && err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "invalid connection token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
			return resp, DisconnectInvalidToken
		}
		if claims, ok := parsedToken.Claims.(*connectTokenClaims); ok && parsedToken.Valid {
			user = claims.StandardClaims.Subject
			info = claims.Info
			b64info = claims.Base64Info
			exp = claims.StandardClaims.ExpiresAt
		} else {
			if validationErr, ok := err.(*jwt.ValidationError); ok {
				if validationErr.Errors == jwt.ValidationErrorExpired {
					// The only problem with token is its expiration - no other
					// errors set in Errors bitfield.
					resp.Error = ErrorTokenExpired
					return resp, nil
				}
				c.node.logger.log(newLogEntry(LogLevelInfo, "invalid connection token", map[string]interface{}{"error": err.Error(), "client": c.uid}))
				return resp, DisconnectInvalidToken
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "invalid connection token", map[string]interface{}{"error": err.Error(), "client": c.uid}))
			return resp, DisconnectInvalidToken
		}

		c.mu.Lock()
		c.user = user
		c.exp = exp
		c.mu.Unlock()

		if len(info) > 0 {
			c.mu.Lock()
			c.info = info
			c.mu.Unlock()
		}
		if b64info != "" {
			byteInfo, err := base64.StdEncoding.DecodeString(b64info)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelInfo, "can not decode provided info from base64", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
				return resp, DisconnectBadRequest
			}
			c.mu.Lock()
			c.info = proto.Raw(byteInfo)
			c.mu.Unlock()
		}
	} else {
		if !insecure && !clientAnonymous {
			c.node.logger.log(newLogEntry(LogLevelInfo, "client credentials not found", map[string]interface{}{"client": c.uid}))
			return resp, DisconnectBadRequest
		}
	}

	c.mu.RLock()
	user := c.user
	exp := c.exp
	c.mu.RUnlock()

	c.node.logger.log(newLogEntry(LogLevelDebug, "client authenticated", map[string]interface{}{"client": c.uid, "user": c.user}))

	if userConnectionLimit > 0 && user != "" && len(c.node.hub.userConnections(user)) >= userConnectionLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "limit of connections for user reached", map[string]interface{}{"user": user, "client": c.uid, "limit": userConnectionLimit}))
		resp.Error = ErrorLimitExceeded
		return resp, nil
	}

	c.mu.RLock()
	if exp > 0 && !insecure {
		expires = true
		now := time.Now().Unix()
		if exp < now {
			c.mu.RUnlock()
			c.node.logger.log(newLogEntry(LogLevelInfo, "connection expiration must be greater than now", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
			resp.Error = ErrorExpired
			return resp, nil
		}
		ttl = uint32(exp - now)
	}
	c.mu.RUnlock()

	res := &proto.ConnectResult{
		Version: version,
		Expires: expires,
		TTL:     ttl,
	}

	resp.Result = res

	// Client successfully connected.
	c.mu.Lock()
	c.authenticated = true
	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}
	c.addPresenceUpdate()
	c.mu.Unlock()

	err := c.node.addClient(c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding client", map[string]interface{}{"client": c.uid, "error": err.Error()}))
		return resp, DisconnectServerError
	}

	if exp > 0 {
		duration := closeDelay + time.Duration(ttl)*time.Second
		c.mu.Lock()
		c.expireTimer = time.AfterFunc(duration, c.expire)
		c.mu.Unlock()
	}

	if c.node.eventHub.refreshHandler != nil {
		// Only require client-side refresh when no refresh handler set.
		resp.Result.Expires = false
		resp.Result.TTL = 0
	}

	resp.Result.Client = c.uid
	if authData != nil {
		resp.Result.Data = authData
	}

	return resp, nil
}

// refreshCmd handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime option set.
func (c *Client) refreshCmd(cmd *proto.RefreshRequest) (*proto.RefreshResponse, *Disconnect) {

	resp := &proto.RefreshResponse{}

	token := cmd.Token
	if token == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client token required to refresh", map[string]interface{}{"user": c.user, "client": c.uid}))
		return resp, DisconnectBadRequest
	}

	config := c.node.Config()
	secret := config.Secret

	var user string
	var expireAt int64
	var info proto.Raw
	var b64info string

	if cmd.Token == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "refresh token required", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
		return resp, DisconnectInvalidToken
	}
	parsedToken, err := jwt.ParseWithClaims(token, &connectTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		if secret == "" {
			return nil, fmt.Errorf("secret not set")
		}
		return []byte(secret), nil
	})
	if parsedToken == nil && err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
		return resp, DisconnectInvalidToken
	}
	if claims, ok := parsedToken.Claims.(*connectTokenClaims); ok && parsedToken.Valid {
		user = claims.StandardClaims.Subject
		info = claims.Info
		b64info = claims.Base64Info
		expireAt = claims.StandardClaims.ExpiresAt
	} else {
		if validationErr, ok := err.(*jwt.ValidationError); ok {
			if validationErr.Errors == jwt.ValidationErrorExpired {
				// The only problem with token is its expiration - no other errors set in bitfield.
				resp.Error = ErrorTokenExpired
				return resp, nil
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
			return resp, DisconnectInvalidToken
		}
		c.node.logger.log(newLogEntry(LogLevelInfo, "invalid refresh token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
		return resp, DisconnectInvalidToken
	}

	res := &proto.RefreshResult{
		Version: config.Version,
		Expires: expireAt > 0,
		Client:  c.uid,
	}

	diff := expireAt - time.Now().Unix()
	if diff > 0 {
		res.TTL = uint32(diff)
	}

	resp.Result = res

	if expireAt > 0 {
		// connection check enabled
		timeToExpire := expireAt - time.Now().Unix()
		if timeToExpire > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.mu.Lock()
			c.exp = expireAt
			if len(info) > 0 {
				c.info = info
			}
			var byteInfo []byte
			if b64info != "" {
				var err error
				byteInfo, err = base64.StdEncoding.DecodeString(b64info)
				if err != nil {
					c.node.logger.log(newLogEntry(LogLevelInfo, "can not decode provided info from base64", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
					return resp, DisconnectBadRequest
				}
				c.info = byteInfo
			}
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(timeToExpire)*time.Second + config.ClientExpiredCloseDelay
			c.expireTimer = time.AfterFunc(duration, c.expire)
			c.mu.Unlock()
		} else {
			resp.Error = ErrorExpired
			return resp, nil
		}
	}
	return resp, nil
}

// subscribeCmd handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel. Optionally we can send missed messages to
// client if it provided last message id seen in channel.
func (c *Client) subscribeCmd(cmd *proto.SubscribeRequest, rw *replyWriter) *Disconnect {

	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for subscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return DisconnectBadRequest
	}

	config := c.node.Config()

	secret := config.Secret
	channelMaxLength := config.ChannelMaxLength
	channelLimit := config.ClientChannelLimit
	insecure := config.ClientInsecure

	res := &proto.SubscribeResult{}

	if channelMaxLength > 0 && len(channel) > channelMaxLength {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel too long", map[string]interface{}{"max": channelMaxLength, "channel": channel, "user": c.user, "client": c.uid}))
		rw.write(&proto.Reply{Error: ErrorLimitExceeded})
		return nil
	}

	c.mu.RLock()
	numChannels := len(c.channels)
	c.mu.RUnlock()

	if channelLimit > 0 && numChannels >= channelLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "maximum limit of channels per client reached", map[string]interface{}{"limit": channelLimit, "user": c.user, "client": c.uid}))
		rw.write(&proto.Reply{Error: ErrorLimitExceeded})
		return nil
	}

	c.mu.RLock()
	_, ok := c.channels[channel]
	c.mu.RUnlock()

	if ok {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already subscribed on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		rw.write(&proto.Reply{Error: ErrorAlreadySubscribed})
		return nil
	}

	if !c.node.userAllowed(channel, c.user) {
		c.node.logger.log(newLogEntry(LogLevelInfo, "user is not allowed to subscribe on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		rw.write(&proto.Reply{Error: ErrorPermissionDenied})
		return nil
	}

	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		rw.write(&proto.Reply{Error: ErrorNamespaceNotFound})
		return nil
	}

	if !chOpts.Anonymous && c.user == "" && !insecure {
		c.node.logger.log(newLogEntry(LogLevelInfo, "anonymous user is not allowed to subscribe on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		rw.write(&proto.Reply{Error: ErrorPermissionDenied})
		return nil
	}

	var channelInfo proto.Raw
	var expireAt int64

	isPrivateChannel := c.node.privateChannel(channel)

	if isPrivateChannel {
		// private channel - subscription request must have valid token.
		var tokenChannel string
		var tokenClient string
		var tokenInfo proto.Raw
		var tokenB64info string

		if cmd.Token == "" {
			c.node.logger.log(newLogEntry(LogLevelInfo, "subscription token required", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
			rw.write(&proto.Reply{Error: ErrorPermissionDenied})
			return nil
		}
		parsedToken, err := jwt.ParseWithClaims(cmd.Token, &subscribeTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			if secret == "" {
				return nil, fmt.Errorf("secret not set")
			}
			return []byte(secret), nil
		})
		if parsedToken == nil && err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
			rw.write(&proto.Reply{Error: ErrorPermissionDenied})
			return nil
		}
		if claims, ok := parsedToken.Claims.(*subscribeTokenClaims); ok && parsedToken.Valid {
			tokenChannel = claims.Channel
			tokenInfo = claims.Info
			tokenB64info = claims.Base64Info
			tokenClient = claims.Client
			expireAt = claims.StandardClaims.ExpiresAt

			if c.uid != tokenClient {
				rw.write(&proto.Reply{Error: ErrorPermissionDenied})
				return nil
			}
			if cmd.Channel != tokenChannel {
				rw.write(&proto.Reply{Error: ErrorPermissionDenied})
				return nil
			}

			if len(tokenInfo) > 0 {
				channelInfo = tokenInfo
			}

			if tokenB64info != "" {
				byteInfo, err := base64.StdEncoding.DecodeString(tokenB64info)
				if err != nil {
					c.node.logger.log(newLogEntry(LogLevelInfo, "can not decode provided info from base64", map[string]interface{}{"user": c.UserID(), "client": c.uid, "error": err.Error()}))
					return DisconnectBadRequest
				}
				channelInfo = byteInfo
			}
		} else {
			if validationErr, ok := err.(*jwt.ValidationError); ok {
				if validationErr.Errors == jwt.ValidationErrorExpired {
					// The only problem with token is its expiration - no other errors set in bitfield.
					rw.write(&proto.Reply{Error: ErrorTokenExpired})
					return nil
				}
				c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
				rw.write(&proto.Reply{Error: ErrorPermissionDenied})
				return nil
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
			rw.write(&proto.Reply{Error: ErrorPermissionDenied})
			return nil
		}
	}

	if c.eventHub.subscribeHandler != nil {
		reply := c.eventHub.subscribeHandler(SubscribeEvent{
			Channel: channel,
		})
		if reply.Disconnect != nil {
			return reply.Disconnect
		}
		if reply.Error != nil {
			rw.write(&proto.Reply{Error: reply.Error})
			return nil
		}
		if len(reply.ChannelInfo) > 0 && !isPrivateChannel {
			channelInfo = reply.ChannelInfo
		}
		if reply.ExpireAt > 0 && !isPrivateChannel {
			expireAt = reply.ExpireAt
		}
	}

	if expireAt > 0 {
		now := time.Now().Unix()
		if expireAt < now {
			c.node.logger.log(newLogEntry(LogLevelInfo, "subscription expiration must be greater than now", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
			rw.write(&proto.Reply{Error: ErrorExpired})
			return nil
		}
		if isPrivateChannel {
			// Only expose expiration info to client in private channel case.
			// In other scenarios expiration will be handled by SubRefreshHandler
			// on Go application backend side.
			res.Expires = true
			res.TTL = uint32(expireAt - now)
		}
	}

	if chOpts.HistoryRecover {
		c.setInSubscribe(channel, true)
	}

	err := c.node.addSubscription(channel, c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		if chOpts.HistoryRecover {
			c.setInSubscribe(channel, false)
		}
		if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
			rw.write(&proto.Reply{Error: clientErr})
			return nil
		}
		return DisconnectServerError
	}

	c.mu.RLock()
	info := c.clientInfo(channel)
	c.mu.RUnlock()

	if chOpts.Presence {
		err = c.node.addPresence(channel, c.uid, info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error adding presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			if chOpts.HistoryRecover {
				c.setInSubscribe(channel, false)
			}
			return DisconnectServerError
		}
	}

	var pubBufferLocked bool

	var latestSeq uint32
	var latestGen uint32
	var latestEpoch string

	var recoveredPubs []*Publication

	if chOpts.HistoryRecover {
		res.Recoverable = true
		if cmd.Recover {
			// Client provided subscribe request with recover flag on. Try to recover missed
			// publications automatically from history (we suppose here that history configured wisely).
			publications, recoveryPosition, err := c.node.recoverHistory(channel, RecoveryPosition{cmd.Seq, cmd.Gen, cmd.Epoch})
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error on recover", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				if chOpts.HistoryRecover {
					c.setInSubscribe(channel, false)
				}
				if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
					rw.write(&proto.Reply{Error: clientErr})
					return nil
				}
				return DisconnectServerError
			}

			latestSeq = recoveryPosition.Seq
			latestGen = recoveryPosition.Gen
			latestEpoch = recoveryPosition.Epoch

			recoveredPubs = publications

			nextSeq, nextGen := nextSeqGen(cmd.Seq, cmd.Gen)
			var recovered bool
			if len(publications) == 0 {
				recovered = latestSeq == cmd.Seq && latestGen == cmd.Gen && latestEpoch == cmd.Epoch
			} else {
				recovered = publications[0].Seq == nextSeq && publications[0].Gen == nextGen && latestEpoch == cmd.Epoch
			}
			res.Recovered = recovered

			recoveredLabel := "no"
			if res.Recovered {
				recoveredLabel = "yes"
			}
			recoverCount.WithLabelValues(recoveredLabel).Inc()
		} else {
			recovery, err := c.node.currentRecoveryState(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error getting recovery state for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				if chOpts.HistoryRecover {
					c.setInSubscribe(channel, false)
				}
				if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
					rw.write(&proto.Reply{Error: clientErr})
					return nil
				}
				return DisconnectServerError
			}
			latestSeq = recovery.Seq
			latestGen = recovery.Gen
			latestEpoch = recovery.Epoch
		}

		res.Epoch = latestEpoch
		res.Seq = latestSeq
		res.Gen = latestGen

		c.pubBufferMu.Lock()
		pubBufferLocked = true
		if len(c.pubBuffer) > 0 {
			recoveredPubs = append(recoveredPubs, c.pubBuffer...)
			c.pubBuffer = nil
		}
		sort.Slice(recoveredPubs, func(i, j int) bool {
			if recoveredPubs[i].Gen != recoveredPubs[j].Gen {
				return recoveredPubs[i].Gen > recoveredPubs[j].Gen
			}
			return recoveredPubs[i].Seq > recoveredPubs[j].Seq
		})
		recoveredPubs = uniquePublications(recoveredPubs)
	}

	res.Publications = recoveredPubs
	replyRes, err := proto.GetResultEncoder(c.transport.Protocol()).EncodeSubscribeResult(res)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error encoding subscribe", map[string]interface{}{"error": err.Error()}))
		if chOpts.HistoryRecover {
			c.setInSubscribe(channel, false)
		}
		return DisconnectServerError
	}
	rw.write(&proto.Reply{Result: replyRes})

	if len(recoveredPubs) > 0 {
		// recoveredPubs are in descending order.
		latestSeq = recoveredPubs[0].Seq
		latestGen = recoveredPubs[0].Gen
	}

	if chOpts.HistoryRecover {
		rw.flush()
		c.setInSubscribe(channel, false)
	}
	if pubBufferLocked {
		c.pubBufferMu.Unlock()
	}

	if chOpts.JoinLeave {
		join := &proto.Join{
			Info: *info,
		}
		rw.flush()
		go c.node.publishJoin(channel, join, &chOpts)
	}

	channelContext := ChannelContext{
		Info:     channelInfo,
		expireAt: expireAt,
		recoveryPosition: RecoveryPosition{
			Seq:   latestSeq,
			Gen:   latestGen,
			Epoch: latestEpoch,
		},
	}
	if chOpts.HistoryRecover {
		channelContext.positionCheckTime = time.Now().Unix()
	}
	c.mu.Lock()
	c.channels[channel] = channelContext
	c.mu.Unlock()

	if c.node.logger.enabled(LogLevelDebug) {
		c.node.logger.log(newLogEntry(LogLevelDebug, "client subscribed to channel", map[string]interface{}{"client": c.uid, "user": c.user, "channel": cmd.Channel}))
	}

	return nil
}

func (c *Client) writePublicationUpdatePosition(ch string, pub *Publication, reply *preparedReply, chOpts *ChannelOptions) error {
	if chOpts.HistoryRecover {
		c.mu.Lock()
		channelContext, ok := c.channels[ch]
		if !ok {
			c.mu.Unlock()
			return nil
		}
		currentPositionSequence := uint64Sequence(channelContext.recoveryPosition.Seq, channelContext.recoveryPosition.Gen)
		nextExpectedSequence := currentPositionSequence + 1
		pubSequence := uint64Sequence(pub.Seq, pub.Gen)
		if pubSequence < nextExpectedSequence {
			// Looks like client already stepped over this sequence – skip sending.
			c.mu.Unlock()
			return nil
		}
		if pubSequence > nextExpectedSequence {
			// Oops: sth lost, let client reconnect to recover missed messages.
			go c.Close(DisconnectInsufficientState)
			c.mu.Unlock()
			return nil
		}
		channelContext.positionCheckTime = time.Now().Unix()
		channelContext.positionCheckFailures = 0
		channelContext.recoveryPosition.Seq = pub.Seq
		channelContext.recoveryPosition.Gen = pub.Gen
		c.channels[ch] = channelContext
		c.mu.Unlock()
	}
	return c.transportSend(reply)
}

func (c *Client) writePublication(ch string, pub *Publication, reply *preparedReply, chOpts *ChannelOptions) error {
	if c.isInSubscribe(ch) {
		// Client currently in process of subscribing to this channel. In this case we keep
		// publications in slice buffer. Publications from this temporary buffer will be sent in
		// subscribe reply.
		c.pubBufferMu.Lock()
		if c.isInSubscribe(ch) {
			c.pubBuffer = append(c.pubBuffer, pub)
		} else {
			c.pubBufferMu.Unlock()
			return c.writePublicationUpdatePosition(ch, pub, reply, chOpts)
		}
		c.pubBufferMu.Unlock()
		return nil
	}
	return c.writePublicationUpdatePosition(ch, pub, reply, chOpts)
}

func (c *Client) writeJoin(ch string, reply *preparedReply) error {
	return c.transportSend(reply)
}

func (c *Client) writeLeave(ch string, reply *preparedReply) error {
	return c.transportSend(reply)
}

func uniquePublications(s []*Publication) []*Publication {
	keys := make(map[uint64]struct{})
	list := []*Publication{}
	for _, entry := range s {
		val := (uint64(entry.Seq))<<32 | uint64(entry.Gen)
		if _, value := keys[val]; !value {
			keys[val] = struct{}{}
			list = append(list, entry)
		}
	}
	return list
}

func (c *Client) subRefreshCmd(cmd *proto.SubRefreshRequest) (*proto.SubRefreshResponse, *Disconnect) {

	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for sub refresh", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &proto.SubRefreshResponse{}
	res := &proto.SubRefreshResult{}

	c.mu.RLock()
	_, ok := c.channels[channel]
	c.mu.RUnlock()
	if !ok {
		// Must be subscribed to refresh.
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	config := c.node.Config()
	secret := config.Secret

	var channelInfo proto.Raw
	var expireAt int64

	var tokenChannel string
	var tokenClient string
	var tokenInfo proto.Raw
	var tokenB64info string

	if cmd.Token == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "subscription refresh token required", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
		resp.Error = ErrorBadRequest
		return resp, nil
	}
	parsedToken, err := jwt.ParseWithClaims(cmd.Token, &subscribeTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		if secret == "" {
			return nil, fmt.Errorf("secret not set")
		}
		return []byte(secret), nil
	})
	if parsedToken == nil && err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
		resp.Error = ErrorBadRequest
		return resp, nil
	}
	if claims, ok := parsedToken.Claims.(*subscribeTokenClaims); ok && parsedToken.Valid {
		tokenChannel = claims.Channel
		tokenInfo = claims.Info
		tokenB64info = claims.Base64Info
		tokenClient = claims.Client
		expireAt = claims.StandardClaims.ExpiresAt
	} else {
		if validationErr, ok := err.(*jwt.ValidationError); ok {
			if validationErr.Errors == jwt.ValidationErrorExpired {
				// The only problem with token is its expiration - no other errors set in bitfield.
				resp.Error = ErrorTokenExpired
				return resp, nil
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
			resp.Error = ErrorBadRequest
			return resp, nil
		}
		c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription refresh token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
		resp.Error = ErrorBadRequest
		return resp, nil
	}

	if c.uid != tokenClient {
		resp.Error = ErrorBadRequest
		return resp, nil
	}
	if cmd.Channel != tokenChannel {
		resp.Error = ErrorBadRequest
		return resp, nil
	}

	if len(tokenInfo) > 0 {
		channelInfo = tokenInfo
	}

	if tokenB64info != "" {
		byteInfo, err := base64.StdEncoding.DecodeString(tokenB64info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "can not decode provided info from base64", map[string]interface{}{"user": c.UserID(), "client": c.uid, "error": err.Error()}))
			return resp, DisconnectBadRequest
		}
		channelInfo = byteInfo
	}

	if expireAt > 0 {
		res.Expires = true
		now := time.Now().Unix()
		if expireAt < now {
			resp.Error = ErrorExpired
			return resp, nil
		}
		res.TTL = uint32(expireAt - now)
	}

	c.mu.Lock()
	channelContext, ok := c.channels[channel]
	if ok {
		channelContext.Info = channelInfo
		channelContext.expireAt = expireAt
	}
	c.channels[channel] = channelContext
	c.mu.Unlock()

	resp.Result = res
	return resp, nil
}

// Lock must be held outside.
func (c *Client) unsubscribe(channel string) error {
	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		return ErrorNamespaceNotFound
	}

	c.mu.RLock()
	info := c.clientInfo(channel)
	_, ok = c.channels[channel]
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
			leave := &proto.Leave{
				Info: *info,
			}
			c.node.publishLeave(channel, leave, &chOpts)
		}

		err := c.node.removeSubscription(channel, c)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error removing subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			return err
		}

		if c.eventHub.unsubscribeHandler != nil {
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

// unsubscribeCmd handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *Client) unsubscribeCmd(cmd *proto.UnsubscribeRequest) (*proto.UnsubscribeResponse, *Disconnect) {

	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for unsubscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &proto.UnsubscribeResponse{}

	err := c.unsubscribe(channel)
	if err != nil {
		if err == ErrorNamespaceNotFound {
			resp.Error = ErrorNamespaceNotFound
		} else {
			resp.Error = ErrorInternal
		}
		return resp, nil
	}

	return resp, nil
}

// publishCmd handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *Client) publishCmd(cmd *proto.PublishRequest) (*proto.PublishResponse, *Disconnect) {

	ch := cmd.Channel
	data := cmd.Data

	if ch == "" || len(data) == 0 {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel and data required for publish", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &proto.PublishResponse{}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		c.node.logger.log(newLogEntry(LogLevelInfo, "attempt to publish to non-existing namespace", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid}))
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	if chOpts.SubscribeToPublish {
		c.mu.RLock()
		_, ok := c.channels[ch]
		c.mu.RUnlock()
		if !ok {
			resp.Error = ErrorPermissionDenied
			return resp, nil
		}
	}

	c.mu.RLock()
	info := c.clientInfo(ch)
	c.mu.RUnlock()

	insecure := c.node.Config().ClientInsecure

	if !chOpts.Publish && !insecure {
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	if c.eventHub.publishHandler != nil {
		reply := c.eventHub.publishHandler(PublishEvent{
			Channel: ch,
			Data:    data,
			Info:    info,
		})
		if reply.Disconnect != nil {
			return resp, reply.Disconnect
		}
		if reply.Error != nil {
			resp.Error = reply.Error
			return resp, nil
		}
		if reply.Data != nil {
			data = reply.Data
		}
	}

	err := c.node.publish(ch, data, info)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error publishing", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp, nil
	}

	return resp, nil
}

// presenceCmd handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *Client) presenceCmd(cmd *proto.PresenceRequest) (*proto.PresenceResponse, *Disconnect) {

	ch := cmd.Channel

	if ch == "" {
		return nil, DisconnectBadRequest
	}

	resp := &proto.PresenceResponse{}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	c.mu.RLock()
	_, ok = c.channels[ch]
	c.mu.RUnlock()

	if !ok {
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	if !chOpts.Presence || chOpts.PresenceDisableForClient {
		resp.Error = ErrorNotAvailable
		return resp, nil
	}

	presence, err := c.node.Presence(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error getting presence", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		if clientErr, ok := err.(*Error); ok {
			resp.Error = clientErr
			return resp, nil
		}
		resp.Error = ErrorInternal
		return resp, nil
	}

	resp.Result = &proto.PresenceResult{
		Presence: presence,
	}

	return resp, nil
}

// presenceStatsCmd handles request to get presence stats – short summary
// about active clients in channel.
func (c *Client) presenceStatsCmd(cmd *proto.PresenceStatsRequest) (*proto.PresenceStatsResponse, *Disconnect) {

	ch := cmd.Channel

	if ch == "" {
		return nil, DisconnectBadRequest
	}

	resp := &proto.PresenceStatsResponse{}

	c.mu.RLock()
	_, ok := c.channels[ch]
	c.mu.RUnlock()

	if !ok {
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	if !chOpts.Presence || chOpts.PresenceDisableForClient {
		resp.Error = ErrorNotAvailable
		return resp, nil
	}

	stats, err := c.node.PresenceStats(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error getting presence stats", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp, nil
	}

	resp.Result = &proto.PresenceStatsResult{
		NumClients: uint32(stats.NumClients),
		NumUsers:   uint32(stats.NumUsers),
	}

	return resp, nil
}

// historyCmd handles history command - it shows last M messages published
// into channel in last N seconds. M is history size and can be configured
// for project or namespace via channel options. N is history lifetime from
// channel options. Both M and N must be set, otherwise this method returns
// ErrorNotAvailable.
func (c *Client) historyCmd(cmd *proto.HistoryRequest) (*proto.HistoryResponse, *Disconnect) {

	ch := cmd.Channel

	if ch == "" {
		return nil, DisconnectBadRequest
	}

	resp := &proto.HistoryResponse{}

	c.mu.RLock()
	_, ok := c.channels[ch]
	c.mu.RUnlock()

	if !ok {
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	if (chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0) || chOpts.HistoryDisableForClient {
		resp.Error = ErrorNotAvailable
		return resp, nil
	}

	pubs, err := c.node.History(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error getting history", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp, nil
	}

	resp.Result = &proto.HistoryResult{
		Publications: pubs,
	}

	return resp, nil
}

// pingCmd handles ping command from client.
func (c *Client) pingCmd(cmd *proto.PingRequest) (*proto.PingResponse, *Disconnect) {
	return &proto.PingResponse{}, nil
}
