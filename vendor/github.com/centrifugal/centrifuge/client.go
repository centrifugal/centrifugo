package centrifuge

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifuge/internal/bufpool"
	"github.com/centrifugal/centrifuge/internal/clientproto"
	"github.com/centrifugal/centrifuge/internal/prepared"
	"github.com/centrifugal/centrifuge/internal/recovery"

	"github.com/centrifugal/protocol"
	"github.com/google/uuid"
)

// We poll current position in channel from history storage periodically.
// If client position is wrong maxCheckPositionFailures times in a row
// then client will be disconnected with InsufficientState reason.
const maxCheckPositionFailures int64 = 2

// channelContext contains extra context for channel connection subscribed to.
type channelContext struct {
	Info                  []byte
	expireAt              int64
	positionCheckTime     int64
	positionCheckFailures int64
	streamPosition        StreamPosition
	serverSide            bool
	clientSideRefresh     bool
}

type timerOp uint8

const (
	timerOpStale    timerOp = 1
	timerOpPresence timerOp = 2
	timerOpExpire   timerOp = 3
)

// Event represent possible client events.
type Event uint64

// Known client events.
const (
	EventConnect Event = 1 << iota
	EventSubscribe
	EventUnsubscribe
	EventPublish
	EventDisconnect
	EventAlive
	EventRefresh
	EventSubRefresh
	EventRPC
	EventMessage
	EventHistory
	EventPresence
	EventPresenceStats
)

// EventAll mask contains all known client events.
const EventAll = EventConnect | EventSubscribe | EventUnsubscribe | EventPublish |
	EventDisconnect | EventAlive | EventRefresh | EventSubRefresh | EventRPC |
	EventMessage | EventHistory | EventPresence | EventPresenceStats

type status uint8

const (
	statusConnecting status = 1
	statusConnected  status = 2
	statusClosed     status = 3
)

// Client represents client connection to server.
type Client struct {
	events            uint64
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
	}

	transportMessagesSentCounter := transportMessagesSent.WithLabelValues(t.Name())

	messageWriterConf := writerConfig{
		MaxQueueSize: n.config.ClientQueueMaxSize,
		WriteFn: func(data []byte) error {
			if err := t.Write(data); err != nil {
				go func() { _ = c.close(DisconnectWriteError) }()
				return err
			}
			transportMessagesSentCounter.Inc()
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
			transportMessagesSentCounter.Add(float64(len(data)))
			return nil
		},
	}

	c.messageWriter = newWriter(messageWriterConf)

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

// Context returns client Context. This context will be canceled when
// client connection closes.
func (c *Client) Context() context.Context {
	return c.ctx
}

func (c *Client) checkSubscriptionExpiration(channel string, channelContext channelContext, delay time.Duration) bool {
	now := c.node.nowTimeGetter().Unix()
	expireAt := channelContext.expireAt
	clientSideRefresh := channelContext.clientSideRefresh
	if expireAt > 0 && now > expireAt+int64(delay.Seconds()) {
		// Subscription expired.
		if !clientSideRefresh && c.node.clientEvents.subRefreshHandler != nil && c.hasEvent(EventSubRefresh) {
			// Give subscription a chance to be refreshed via SubRefreshHandler.
			reply, err := c.node.clientEvents.subRefreshHandler(c, SubRefreshEvent{Channel: channel})
			if err != nil {
				return false
			}
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
	if c.node.clientEvents.aliveHandler != nil && c.hasEvent(EventAlive) {
		c.node.clientEvents.aliveHandler(c)
	}
	for channel, channelContext := range channels {
		if !c.checkSubscriptionExpiration(channel, channelContext, config.ClientExpiredSubCloseDelay) {
			// Ideally we should deal with single expired subscription in this
			// case - i.e. unsubscribe client from channel and give an advice
			// to resubscribe. But there is scenario when browser goes online
			// after computer was in sleeping mode which I have not managed to
			// handle reliably on client side when unsubscribe with resubscribe
			// flag was used. So I decided to stick with disconnect for now -
			// it seems to work fine and drastically simplifies client code.
			go func() { _ = c.close(DisconnectSubExpired) }()
			// No need to proceed after close.
			return
		}

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

// UserID returns user ID associated with client connection.
func (c *Client) UserID() string {
	return c.user
}

// Transport returns transport details used by client connection.
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
// already closed. Since this method runs a separate goroutine client
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
		serverDisconnectCount.WithLabelValues(strconv.FormatUint(uint64(disconnect.Code), 10)).Inc()
	}
	if c.node.clientEvents.disconnectHandler != nil && c.hasEvent(EventDisconnect) && prevStatus == statusConnected {
		c.node.clientEvents.disconnectHandler(c, DisconnectEvent{
			Disconnect: disconnect,
		})
	}
	return nil
}

func (c *Client) hasEvent(e Event) bool {
	return Event(atomic.LoadUint64(&c.events))&e != 0
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

// Handle raw data encoded with Centrifuge protocol. Not goroutine-safe.
func (c *Client) Handle(data []byte) bool {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()

	if len(data) == 0 {
		c.node.logger.log(newLogEntry(LogLevelError, "empty client request received", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
		_ = c.close(DisconnectBadRequest)
		return false
	}

	protoType := c.transport.Protocol().toProto()

	encoder := protocol.GetReplyEncoder(protoType)
	decoder := protocol.GetCommandDecoder(protoType, data)

	for {
		cmd, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding command", map[string]interface{}{"data": string(data), "client": c.ID(), "user": c.UserID(), "error": err.Error()}))
			_ = c.close(DisconnectBadRequest)
			protocol.PutCommandDecoder(protoType, decoder)
			protocol.PutReplyEncoder(protoType, encoder)
			return false
		}
		var encodeErr error
		write := func(rep *protocol.Reply) error {
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
					protocol.PutCommandDecoder(protoType, decoder)
					protocol.PutReplyEncoder(protoType, encoder)
					_ = c.close(disconnect)
					return fmt.Errorf("flush error")
				}
			}
			encoder.Reset()
			return nil
		}
		disconnect := c.handleCommand(cmd, write, flush)
		select {
		case <-c.ctx.Done():
			if encodeErr == nil {
				protocol.PutCommandDecoder(protoType, decoder)
				protocol.PutReplyEncoder(protoType, encoder)
			}
			return true
		default:
		}
		if disconnect != nil {
			if disconnect != DisconnectNormal {
				c.node.logger.log(newLogEntry(LogLevelInfo, "disconnect after handling command", map[string]interface{}{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
			}
			_ = c.close(disconnect)
			if encodeErr == nil {
				protocol.PutCommandDecoder(protoType, decoder)
				protocol.PutReplyEncoder(protoType, encoder)
			}
			return false
		}
		if encodeErr != nil {
			_ = c.close(DisconnectServerError)
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
			_ = c.close(disconnect)
			protocol.PutCommandDecoder(protoType, decoder)
			protocol.PutReplyEncoder(protoType, encoder)
			return false
		}
	}

	protocol.PutCommandDecoder(protoType, decoder)
	protocol.PutReplyEncoder(protoType, encoder)

	return true
}

type replyWriter struct {
	write func(*protocol.Reply) error
	flush func() error
}

// handle dispatches Command into correct command handler.
func (c *Client) handleCommand(cmd *protocol.Command, writeFn func(*protocol.Reply) error, flush func() error) *Disconnect {
	c.mu.Lock()
	if c.status == statusClosed {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	var disconnect *Disconnect

	method := cmd.Method
	params := cmd.Params

	write := func(rep *protocol.Reply) error {
		rep.ID = cmd.ID
		if rep.Error != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "client command error", map[string]interface{}{"reply": fmt.Sprintf("%v", rep), "command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "error": rep.Error.Error()}))
			replyErrorCount.WithLabelValues(strings.ToLower(protocol.MethodType_name[int32(method)]), strconv.FormatUint(uint64(rep.Error.Code), 10)).Inc()
		}
		return writeFn(rep)
	}

	rw := &replyWriter{write, flush}

	if cmd.ID == 0 && method != protocol.MethodTypeSend {
		c.node.logger.log(newLogEntry(LogLevelInfo, "command ID required for commands with reply expected", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
		_ = rw.write(&protocol.Reply{Error: ErrorBadRequest.toProto()})
		return nil
	}

	if method != protocol.MethodTypeConnect && !c.authenticated {
		// Client must send connect command first.
		_ = rw.write(&protocol.Reply{Error: ErrorUnauthorized.toProto()})
		return nil
	}

	started := time.Now()
	switch method {
	case protocol.MethodTypeConnect:
		disconnect = c.handleConnect(params, rw)
	case protocol.MethodTypeRefresh:
		disconnect = c.handleRefresh(params, rw)
	case protocol.MethodTypeSubscribe:
		disconnect = c.handleSubscribe(params, rw)
	case protocol.MethodTypeSubRefresh:
		disconnect = c.handleSubRefresh(params, rw)
	case protocol.MethodTypeUnsubscribe:
		disconnect = c.handleUnsubscribe(params, rw)
	case protocol.MethodTypePublish:
		disconnect = c.handlePublish(params, rw)
	case protocol.MethodTypePresence:
		disconnect = c.handlePresence(params, rw)
	case protocol.MethodTypePresenceStats:
		disconnect = c.handlePresenceStats(params, rw)
	case protocol.MethodTypeHistory:
		disconnect = c.handleHistory(params, rw)
	case protocol.MethodTypePing:
		disconnect = c.handlePing(params, rw)
	case protocol.MethodTypeRPC:
		disconnect = c.handleRPC(params, rw)
	case protocol.MethodTypeSend:
		disconnect = c.handleSend(params)
	default:
		_ = rw.write(&protocol.Reply{Error: ErrorMethodNotFound.toProto()})
	}
	commandDurationSummary.WithLabelValues(strings.ToLower(protocol.MethodType_name[int32(method)])).Observe(time.Since(started).Seconds())
	return disconnect
}

func (c *Client) expire() {
	c.mu.RLock()
	closed := c.status == statusClosed
	clientSideRefresh := c.clientSideRefresh
	c.mu.RUnlock()
	if closed {
		return
	}

	now := time.Now().Unix()

	if !clientSideRefresh && c.node.clientEvents.refreshHandler != nil && c.hasEvent(EventRefresh) {
		reply, err := c.node.clientEvents.refreshHandler(c, RefreshEvent{})
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
	}

	c.mu.RLock()
	exp := c.exp
	c.mu.RUnlock()

	if exp == 0 {
		return
	}

	ttl := exp - now

	if !clientSideRefresh && c.node.clientEvents.refreshHandler != nil && c.hasEvent(EventRefresh) {
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

func (c *Client) handleConnect(params protocol.Raw, rw *replyWriter) *Disconnect {
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
	if c.node.clientEvents.connectHandler == nil || !c.hasEvent(EventConnect) {
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

func (c *Client) handleRefresh(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding refresh", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.refreshCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeRefreshResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding refresh", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleSubscribe(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeSubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding subscribe", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	ctx := c.subscribeCmd(cmd, rw, false)
	if ctx.disconnect != nil {
		return ctx.disconnect
	}
	if ctx.err != nil {
		return nil
	}
	if ctx.chOpts.JoinLeave && ctx.clientInfo != nil {
		// Flush prevents Join message to be delivered before subscribe response.
		_ = rw.flush()
		go func() { _ = c.node.publishJoin(cmd.Channel, ctx.clientInfo, &ctx.chOpts) }()
	}
	return nil
}

func (c *Client) handleSubRefresh(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeSubRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding sub refresh", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.subRefreshCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeSubRefreshResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding sub refresh", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleUnsubscribe(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeUnsubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding unsubscribe", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.unsubscribeCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeUnsubscribeResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding unsubscribe", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePublish(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePublish(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding publish", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.publishCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodePublishResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding publish", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePresence(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePresence(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.presenceCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodePresenceResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePresenceStats(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePresenceStats(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence stats", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.presenceStatsCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodePresenceStatsResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence stats", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleHistory(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeHistory(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding history", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.historyCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeHistoryResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding history", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handlePing(params protocol.Raw, rw *replyWriter) *Disconnect {
	cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodePing(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding ping", map[string]interface{}{"error": err.Error()}))
		return DisconnectBadRequest
	}
	resp, disconnect := c.pingCmd(cmd)
	if disconnect != nil {
		return disconnect
	}
	if resp.Error != nil {
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodePingResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding ping", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	return nil
}

func (c *Client) handleRPC(params protocol.Raw, rw *replyWriter) *Disconnect {
	if c.node.clientEvents.rpcHandler != nil && c.hasEvent(EventRPC) {
		cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeRPC(params)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding rpc", map[string]interface{}{"error": err.Error()}))
			return DisconnectBadRequest
		}
		rpcReply, err := c.node.clientEvents.rpcHandler(c, RPCEvent{
			Method: cmd.Method,
			Data:   cmd.Data,
		})
		if err != nil {
			switch t := err.(type) {
			case *Disconnect:
				return t
			default:
				_ = rw.write(&protocol.Reply{Error: toClientErr(err).toProto()})
				return nil
			}
		}

		result := &protocol.RPCResult{
			Data: rpcReply.Data,
		}

		var replyRes []byte
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeRPCResult(result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding rpc", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
		_ = rw.write(&protocol.Reply{Result: replyRes})
		return nil
	}
	_ = rw.write(&protocol.Reply{Error: ErrorNotAvailable.toProto()})
	return nil
}

func (c *Client) handleSend(params protocol.Raw) *Disconnect {
	if c.node.clientEvents.messageHandler != nil && c.hasEvent(EventMessage) {
		cmd, err := protocol.GetParamsDecoder(c.transport.Protocol().toProto()).DecodeSend(params)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding message", map[string]interface{}{"error": err.Error()}))
			return DisconnectBadRequest
		}
		c.node.clientEvents.messageHandler(c, MessageEvent{
			Data: cmd.Data,
		})
		return nil
	}
	return nil
}

func (c *Client) unlockServerSideSubscriptions(subCtxMap map[string]subscribeContext) {
	for channel := range subCtxMap {
		c.pubSubSync.StopBuffering(channel)
	}
}

// connectCmd handles connect command from client - client must send connect
// command immediately after establishing connection with server.
func (c *Client) connectCmd(cmd *protocol.ConnectRequest, rw *replyWriter) *Disconnect {
	resp := &clientproto.ConnectResponse{}

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

	events := EventAll

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
			switch t := err.(type) {
			case *Disconnect:
				return t
			default:
				_ = rw.write(&protocol.Reply{Error: toClientErr(err).toProto()})
				return nil
			}
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
		if reply.Events > 0 {
			events = reply.Events
		}
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

	atomic.StoreUint64(&c.events, uint64(events))

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
		resp.Error = ErrorLimitExceeded.toProto()
		_ = rw.write(&protocol.Reply{Error: resp.Error})
		return nil
	}

	c.mu.RLock()
	if exp > 0 {
		expires = true
		now := time.Now().Unix()
		if exp < now {
			c.mu.RUnlock()
			c.node.logger.log(newLogEntry(LogLevelInfo, "connection expiration must be greater than now", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
			resp.Error = ErrorExpired.toProto()
			_ = rw.write(&protocol.Reply{Error: resp.Error})
			return nil
		}
		ttl = uint32(exp - now)
	}
	c.mu.RUnlock()

	res := &protocol.ConnectResult{
		Version: version,
		Expires: expires,
		TTL:     ttl,
	}

	resp.Result = res

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
		resp.Result.Expires = false
		resp.Result.TTL = 0
	}

	resp.Result.Client = c.uid
	if authData != nil {
		resp.Result.Data = authData
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
				subCtx := c.subscribeCmd(subCmd, rw, true)
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
			_ = rw.write(&protocol.Reply{Error: subError.toProto()})
			return nil
		}
		if resp.Result != nil {
			resp.Result.Subs = subs
		}
	}

	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = protocol.GetResultEncoder(c.transport.Protocol().toProto()).EncodeConnectResult(resp.Result)
		if err != nil {
			c.unlockServerSideSubscriptions(subCtxMap)
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding connect", map[string]interface{}{"error": err.Error()}))
			return DisconnectServerError
		}
	}
	_ = rw.write(&protocol.Reply{Result: replyRes})
	_ = rw.flush()

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
					_ = c.node.publishJoin(channel, subCtx.clientInfo, &subCtx.chOpts)
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
	subCtx := c.subscribeCmd(subCmd, nil, true)
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

// refreshCmd handles refresh command from client to confirm the actual state of connection.
func (c *Client) refreshCmd(cmd *protocol.RefreshRequest) (*clientproto.RefreshResponse, *Disconnect) {
	resp := &clientproto.RefreshResponse{}

	if cmd.Token == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client token required to refresh", map[string]interface{}{"user": c.user, "client": c.uid}))
		return resp, DisconnectBadRequest
	}

	config := c.node.config

	c.mu.RLock()
	clientSideRefresh := c.clientSideRefresh
	c.mu.RUnlock()

	if !clientSideRefresh {
		// Client not supposed to send refresh command in case of server-side refresh mechanism.
		return resp, DisconnectBadRequest
	}

	var expireAt int64
	var info []byte

	if c.node.clientEvents.refreshHandler != nil && c.hasEvent(EventRefresh) {
		reply, err := c.node.clientEvents.refreshHandler(c, RefreshEvent{
			ClientSideRefresh: true,
			Token:             cmd.Token,
		})
		if err != nil {
			switch t := err.(type) {
			case *Disconnect:
				return resp, t
			default:
				resp.Error = toClientErr(err).toProto()
				return resp, nil
			}
		}
		if reply.Expired {
			return resp, DisconnectExpired
		}
		expireAt = reply.ExpireAt
		info = reply.Info
	} else {
		return resp, DisconnectExpired
	}

	res := &protocol.RefreshResult{
		Version: config.Version,
		Expires: expireAt > 0,
		Client:  c.uid,
	}

	ttl := expireAt - time.Now().Unix()

	if ttl > 0 {
		res.TTL = uint32(ttl)
	}

	resp.Result = res

	if expireAt > 0 {
		// connection check enabled
		if ttl > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.mu.Lock()
			c.exp = expireAt
			if len(info) > 0 {
				c.info = info
			}
			duration := time.Duration(ttl)*time.Second + config.ClientExpiredCloseDelay
			c.addExpireUpdate(duration)
			c.mu.Unlock()
		} else {
			resp.Error = ErrorExpired.toProto()
			return resp, nil
		}
	}
	return resp, nil
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

func (c *Client) extractSubscribeData(cmd *protocol.SubscribeRequest, serverSide bool) (protocol.Raw, int64, bool, *Error, *Disconnect) {
	var (
		channelInfo protocol.Raw
		expireAt    int64
	)

	var clientSideRefresh bool

	if !serverSide {
		if c.node.clientEvents.subscribeHandler != nil && c.hasEvent(EventSubscribe) {
			reply, err := c.node.clientEvents.subscribeHandler(c, SubscribeEvent{
				Channel: cmd.Channel,
				Token:   cmd.Token,
			})
			if err != nil {
				switch t := err.(type) {
				case *Disconnect:
					return nil, 0, false, nil, t
				default:
					return nil, 0, false, toClientErr(err), nil
				}
			}
			if len(reply.ChannelInfo) > 0 {
				channelInfo = reply.ChannelInfo
			}
			if reply.ExpireAt > 0 {
				expireAt = reply.ExpireAt
			}
			clientSideRefresh = reply.ClientSideRefresh
		} else {
			return nil, 0, false, ErrorNotAvailable, nil
		}
	}

	return channelInfo, expireAt, clientSideRefresh, nil, nil
}

func handleErrorDisconnect(rw *replyWriter, replyError *Error, disconnect *Disconnect, serverSide bool) subscribeContext {
	ctx := subscribeContext{}
	if disconnect != nil {
		ctx.disconnect = disconnect
		return ctx
	}
	if replyError != nil {
		if !serverSide {
			// Subscription request not from client - no need to send
			// response to connection.
			_ = rw.write(&protocol.Reply{Error: replyError.toProto()})
		}
		ctx.err = replyError
		return ctx
	}
	return ctx
}

func incRecoverCount(recovered bool) {
	recoveredLabel := "no"
	if recovered {
		recoveredLabel = "yes"
	}
	recoverCount.WithLabelValues(recoveredLabel).Inc()
}

type subscribeContext struct {
	result         *protocol.SubscribeResult
	chOpts         ChannelOptions
	clientInfo     *ClientInfo
	err            *Error
	disconnect     *Disconnect
	channelContext channelContext
}

// subscribeCmd handles subscribe command - clients send this when subscribe
// on channel, if channel if private then we must validate provided sign here before
// actually subscribe client on channel. Optionally we can send missed messages to
// client if it provided last message id seen in channel.
func (c *Client) subscribeCmd(cmd *protocol.SubscribeRequest, rw *replyWriter, serverSide bool) subscribeContext {
	chOpts, replyError, disconnect := c.validateSubscribeRequest(cmd)
	if disconnect != nil || replyError != nil {
		return handleErrorDisconnect(rw, replyError, disconnect, serverSide)
	}

	channelInfo, expireAt, clientSideRefresh, replyError, disconnect := c.extractSubscribeData(cmd, serverSide)
	if disconnect != nil || replyError != nil {
		return handleErrorDisconnect(rw, replyError, disconnect, serverSide)
	}

	ctx := subscribeContext{}
	res := &protocol.SubscribeResult{}

	if expireAt > 0 {
		ttl := expireAt - time.Now().Unix()
		if ttl <= 0 {
			c.node.logger.log(newLogEntry(LogLevelInfo, "subscription expiration must be greater than now", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
			return handleErrorDisconnect(rw, ErrorExpired, nil, serverSide)
		}
		if clientSideRefresh {
			res.Expires = true
			res.TTL = uint32(ttl)
		}
	}

	channel := cmd.Channel

	info := &ClientInfo{
		ClientID: c.uid,
		UserID:   c.user,
		ConnInfo: c.info,
		ChanInfo: channelInfo,
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
			return handleErrorDisconnect(rw, clientErr, nil, serverSide)
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
					return handleErrorDisconnect(rw, clientErr, nil, serverSide)
				}
				ctx.disconnect = DisconnectServerError
				return ctx
			}

			latestOffset = historyResult.Offset
			latestEpoch = historyResult.Epoch

			recoveredPubs = make([]*protocol.Publication, 0, len(historyResult.Publications))
			for _, pub := range historyResult.Publications {
				protoPub := pubToProto(pub)
				if useSeqGen {
					protoPub.Seq, protoPub.Gen = recovery.UnpackUint64(protoPub.Offset)
				}
				recoveredPubs = append(recoveredPubs, protoPub)
			}

			nextOffset := cmdOffset + 1
			var recovered bool
			if len(recoveredPubs) == 0 {
				recovered = latestOffset == cmdOffset && latestEpoch == cmd.Epoch
			} else {
				recovered = recoveredPubs[0].Offset == nextOffset && latestEpoch == cmd.Epoch
			}
			res.Recovered = recovered
			incRecoverCount(res.Recovered)
		} else {
			streamTop, err := c.node.streamTop(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error getting recovery state for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				c.pubSubSync.StopBuffering(channel)
				if clientErr, ok := err.(*Error); ok && clientErr != ErrorInternal {
					return handleErrorDisconnect(rw, clientErr, nil, serverSide)
				}
				ctx.disconnect = DisconnectServerError
				return ctx
			}
			latestOffset = streamTop.Offset
			latestEpoch = streamTop.Epoch
		}

		res.Epoch = latestEpoch
		res.Offset = latestOffset

		if useSeqGen {
			res.Seq, res.Gen = recovery.UnpackUint64(latestOffset)
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

	res.Publications = recoveredPubs
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
		_ = rw.write(&protocol.Reply{Result: replyRes})
	}

	if len(recoveredPubs) > 0 {
		if useSeqGen {
			// recoveredPubs are in descending order.
			latestOffset = recoveredPubs[0].Offset
		} else {
			latestOffset = recoveredPubs[len(recoveredPubs)-1].Offset
		}
	}

	if !serverSide && chOpts.HistoryRecover {
		// Need to flush data from writer so subscription response is
		// sent before any subscription publication.
		_ = rw.flush()
	}

	channelContext := channelContext{
		Info:              channelInfo,
		serverSide:        serverSide,
		clientSideRefresh: clientSideRefresh,
		expireAt:          expireAt,
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

func (c *Client) subRefreshCmd(cmd *protocol.SubRefreshRequest) (*clientproto.SubRefreshResponse, *Disconnect) {
	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for sub refresh", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &clientproto.SubRefreshResponse{}

	if cmd.Token == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "subscription refresh token required", map[string]interface{}{"client": c.uid, "user": c.UserID()}))
		resp.Error = ErrorBadRequest.toProto()
		return resp, nil
	}

	c.mu.RLock()
	ctx, okChannel := c.channels[channel]
	clientSideRefresh := ctx.clientSideRefresh
	c.mu.RUnlock()
	if !okChannel {
		// Must be subscribed to refresh.
		resp.Error = ErrorPermissionDenied.toProto()
		return resp, nil
	}

	if !clientSideRefresh {
		// Client not supposed to send sub refresh command in case of server-side
		// subscription refresh mechanism.
		return resp, DisconnectBadRequest
	}

	res := &protocol.SubRefreshResult{}

	var expireAt int64
	var info []byte

	if c.node.clientEvents.subRefreshHandler != nil && c.hasEvent(EventSubRefresh) {
		reply, err := c.node.clientEvents.subRefreshHandler(c, SubRefreshEvent{
			ClientSideRefresh: true,
			Channel:           cmd.Channel,
			Token:             cmd.Token,
		})
		if err != nil {
			switch t := err.(type) {
			case *Disconnect:
				return resp, t
			default:
				resp.Error = toClientErr(err).toProto()
				return resp, nil
			}
		}
		if reply.Expired {
			return resp, DisconnectExpired
		}
		expireAt = reply.ExpireAt
		info = reply.Info
	} else {
		return resp, DisconnectExpired
	}

	if expireAt > 0 {
		res.Expires = true
		now := time.Now().Unix()
		if expireAt < now {
			resp.Error = ErrorExpired.toProto()
			return resp, nil
		}
		res.TTL = uint32(expireAt - now)
	}

	c.mu.Lock()
	channelContext, okChan := c.channels[channel]
	if okChan {
		channelContext.Info = info
		channelContext.expireAt = expireAt
	}
	c.channels[channel] = channelContext
	c.mu.Unlock()

	resp.Result = res
	return resp, nil
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
			_ = c.node.publishLeave(channel, info, &chOpts)
		}

		if err := c.node.removeSubscription(channel, c); err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error removing subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			return err
		}

		if !serverSide && c.node.clientEvents.unsubscribeHandler != nil && c.hasEvent(EventUnsubscribe) {
			c.node.clientEvents.unsubscribeHandler(c, UnsubscribeEvent{
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
// unsubscribe connection from channel.
func (c *Client) unsubscribeCmd(cmd *protocol.UnsubscribeRequest) (*clientproto.UnsubscribeResponse, *Disconnect) {
	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for unsubscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &clientproto.UnsubscribeResponse{}

	if err := c.unsubscribe(channel); err != nil {
		resp.Error = toClientErr(err).toProto()
		return resp, nil
	}

	return resp, nil
}

// publishCmd handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options.
func (c *Client) publishCmd(cmd *protocol.PublishRequest) (*clientproto.PublishResponse, *Disconnect) {
	ch := cmd.Channel
	data := cmd.Data

	if ch == "" || len(data) == 0 {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel and data required for publish", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &clientproto.PublishResponse{}

	c.mu.RLock()
	info := c.clientInfo(ch)
	c.mu.RUnlock()

	var publishResult *PublishResult

	if c.node.clientEvents.publishHandler != nil && c.hasEvent(EventPublish) {
		reply, err := c.node.clientEvents.publishHandler(c, PublishEvent{
			Channel: ch,
			Data:    data,
			Info:    info,
		})
		if err != nil {
			switch t := err.(type) {
			case *Disconnect:
				return resp, t
			default:
				resp.Error = toClientErr(err).toProto()
				return resp, nil
			}
		}
		publishResult = reply.Result
	} else {
		resp.Error = ErrorNotAvailable.toProto()
		return resp, nil
	}

	if publishResult == nil {
		_, err := c.node.publish(ch, data, info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error publishing", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
			resp.Error = ErrorInternal.toProto()
			return resp, nil
		}
	}

	return resp, nil
}

func toClientErr(err error) *Error {
	if clientErr, ok := err.(*Error); ok {
		return clientErr
	}
	return ErrorInternal
}

// presenceCmd handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *Client) presenceCmd(cmd *protocol.PresenceRequest) (*clientproto.PresenceResponse, *Disconnect) {
	ch := cmd.Channel
	if ch == "" {
		return nil, DisconnectBadRequest
	}

	resp := &clientproto.PresenceResponse{}

	chOpts, found, err := c.node.channelOptions(ch)
	if err != nil {
		resp.Error = toClientErr(err).toProto()
		return resp, nil
	}
	if !found {
		resp.Error = ErrorUnknownChannel.toProto()
		return resp, nil
	}
	if !chOpts.Presence || c.node.clientEvents.presenceHandler == nil || !c.hasEvent(EventPresence) {
		resp.Error = ErrorNotAvailable.toProto()
		return resp, nil
	}

	_, err = c.node.clientEvents.presenceHandler(c, PresenceEvent{
		Channel: ch,
	})
	if err != nil {
		switch t := err.(type) {
		case *Disconnect:
			return resp, t
		default:
			resp.Error = toClientErr(err).toProto()
			return resp, nil
		}
	}

	result, err := c.node.Presence(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(errLogLevel(err), "error getting presence", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = toClientErr(err).toProto()
		return resp, nil
	}

	presence := make(map[string]*protocol.ClientInfo, len(result.Presence))
	for k, v := range result.Presence {
		presence[k] = infoToProto(v)
	}

	resp.Result = &protocol.PresenceResult{
		Presence: presence,
	}

	return resp, nil
}

// presenceStatsCmd handles request to get presence stats – short summary
// about active clients in channel.
func (c *Client) presenceStatsCmd(cmd *protocol.PresenceStatsRequest) (*clientproto.PresenceStatsResponse, *Disconnect) {
	ch := cmd.Channel
	if ch == "" {
		return nil, DisconnectBadRequest
	}

	resp := &clientproto.PresenceStatsResponse{}

	chOpts, found, err := c.node.channelOptions(ch)
	if err != nil {
		resp.Error = toClientErr(err).toProto()
		return resp, nil
	}
	if !found {
		resp.Error = ErrorUnknownChannel.toProto()
		return resp, nil
	}
	if !chOpts.Presence || c.node.clientEvents.presenceStatsHandler == nil || !c.hasEvent(EventPresenceStats) {
		resp.Error = ErrorNotAvailable.toProto()
		return resp, nil
	}

	_, err = c.node.clientEvents.presenceStatsHandler(c, PresenceStatsEvent{
		Channel: ch,
	})
	if err != nil {
		switch t := err.(type) {
		case *Disconnect:
			return resp, t
		default:
			resp.Error = toClientErr(err).toProto()
			return resp, nil
		}
	}

	stats, err := c.node.PresenceStats(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(errLogLevel(err), "error getting presence stats", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = ErrorInternal.toProto()
		return resp, nil
	}

	resp.Result = &protocol.PresenceStatsResult{
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
func (c *Client) historyCmd(cmd *protocol.HistoryRequest) (*clientproto.HistoryResponse, *Disconnect) {
	ch := cmd.Channel
	if ch == "" {
		return nil, DisconnectBadRequest
	}

	resp := &clientproto.HistoryResponse{}

	chOpts, found, err := c.node.channelOptions(ch)
	if err != nil {
		resp.Error = toClientErr(err).toProto()
		return resp, nil
	}
	if !found {
		resp.Error = ErrorUnknownChannel.toProto()
		return resp, nil
	}
	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 || c.node.clientEvents.historyHandler == nil || !c.hasEvent(EventHistory) {
		resp.Error = ErrorNotAvailable.toProto()
		return resp, nil
	}

	_, err = c.node.clientEvents.historyHandler(c, HistoryEvent{
		Channel: ch,
	})
	if err != nil {
		switch t := err.(type) {
		case *Disconnect:
			return resp, t
		default:
			resp.Error = toClientErr(err).toProto()
			return resp, nil
		}
	}

	historyResult, err := c.node.fullHistory(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(errLogLevel(err), "error getting history", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = ErrorInternal.toProto()
		return resp, nil
	}

	pubs := make([]*protocol.Publication, 0, len(historyResult.Publications))
	for _, pub := range historyResult.Publications {
		protoPub := pubToProto(pub)
		if hasFlag(CompatibilityFlags, UseSeqGen) {
			protoPub.Seq, protoPub.Gen = recovery.UnpackUint64(protoPub.Offset)
		}
		pubs = append(pubs, protoPub)
	}

	resp.Result = &protocol.HistoryResult{
		Publications: pubs,
	}

	return resp, nil
}

// pingCmd handles ping command from client.
func (c *Client) pingCmd(_ *protocol.PingRequest) (*clientproto.PingResponse, *Disconnect) {
	return &clientproto.PingResponse{}, nil
}

func errLogLevel(err error) LogLevel {
	logLevel := LogLevelInfo
	if err != ErrorNotAvailable {
		logLevel = LogLevelError
	}
	return logLevel
}
