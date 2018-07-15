package centrifuge

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
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
	refreshHandler     RefreshHandler
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

// Refresh allows to set RefreshHandler.
// RefreshHandler called when it's time to refresh client connection credentials.
func (c *ClientEventHub) Refresh(h RefreshHandler) {
	c.refreshHandler = h
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

// Credentials allows to authenticate connection when set into context.
type Credentials struct {
	UserID   string
	ExpireAt int64
	Info     []byte
}

// credentialsContextKeyType is special type to safely use
// context for setting and getting Credentials.
type credentialsContextKeyType int

// CredentialsContextKey allows Go code to set Credentials into context.
var credentialsContextKey credentialsContextKeyType

// SetCredentials allows to set connection Credentials to context.
func SetCredentials(ctx context.Context, creds *Credentials) context.Context {
	ctx = context.WithValue(ctx, credentialsContextKey, creds)
	return ctx
}

// ChannelContext contains extra context for channel connection subscribed to.
type ChannelContext struct {
	Info     proto.Raw
	expireAt int64
}

// Client represents client connection to server.
type Client struct {
	mu sync.RWMutex

	ctx       context.Context
	node      *Node
	transport transport

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
}

// newClient initializes new Client.
func newClient(ctx context.Context, n *Node, t transport) (*Client, error) {
	uuidObject, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	c := &Client{
		ctx:       ctx,
		uid:       uuidObject.String(),
		node:      n,
		transport: t,
		eventHub:  &ClientEventHub{},
	}

	config := n.Config()
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
		c.close(DisconnectStale)
	}
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect
func (c *Client) updateChannelPresence(ch string) error {
	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		return ErrorNamespaceNotFound
	}
	if !chOpts.Presence {
		return nil
	}
	info := c.clientInfo(ch)
	return c.node.addPresence(ch, c.uid, info)
}

// updatePresence updates presence info for all client channels.
// At moment it also checks for expired subscriptions. As this happens
// once in configured presence ping interval then subscription
// expiration time resolution is pretty big. Though on practice
// this should be reasonable for most use cases.
func (c *Client) updatePresence() {
	config := c.node.Config()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	for channel, channelContext := range c.channels {
		now := time.Now().Unix()
		expireAt := channelContext.expireAt
		if expireAt > 0 && now > expireAt+int64(config.ClientExpiredSubCloseDelay.Seconds()) {
			// Subscription expired.
			if c.eventHub.subRefreshHandler != nil {
				// Give subscription a chance to be refreshed via SubRefreshHandler.
				reply := c.eventHub.subRefreshHandler(SubRefreshEvent{Channel: channel})
				if reply.Expired || (reply.ExpireAt > 0 && reply.ExpireAt < now) {
					go c.Unsubscribe(channel, true)
					// No need to update channel presence.
					continue
				}
				ctx := c.channels[channel]
				if len(reply.Info) > 0 {
					ctx.Info = reply.Info
				}
				ctx.expireAt = reply.ExpireAt
				c.channels[channel] = ctx
			} else {
				// The only way subscription could be refreshed in this case is via
				// SUB_REFRESH command sent from client but looks like that command
				// with new refreshed token have not been received in configured window.
				go c.Unsubscribe(channel, true)
				// No need to update channel presence.
				continue
			}
		}

		err := c.updateChannelPresence(channel)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error updating presence for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		}
	}
	c.addPresenceUpdate()
}

// Lock must be held outside.
func (c *Client) addPresenceUpdate() {
	config := c.node.Config()
	presenceInterval := config.ClientPresencePingInterval
	c.presenceTimer = time.AfterFunc(presenceInterval, c.updatePresence)
}

// ID returns unique client connection id.
func (c *Client) ID() string {
	return c.uid
}

// UserID returns user ID associated with client connection.
func (c *Client) UserID() string {
	return c.user
}

// Transport returns transport used by client connection.
func (c *Client) Transport() Transport {
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

	pushEncoder := proto.GetPushEncoder(c.transport.Encoding())
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
	}, c.transport.Encoding())

	return c.transport.Send(reply)
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
	c.sendUnsub(ch, resubscribe)
	return nil
}

func (c *Client) sendUnsub(ch string, resubscribe bool) error {
	pushEncoder := proto.GetPushEncoder(c.transport.Encoding())

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
	}, c.transport.Encoding())

	c.transport.Send(reply)

	return nil
}

// Close closes client connection.
func (c *Client) Close(disconnect *Disconnect) error {
	c.mu.Lock()
	c.disconnect = disconnect
	c.mu.Unlock()
	// Closed transport will cause transport connection handler to
	// return and thus calling client's close method.
	return c.transport.Close(disconnect)
}

// close client connection with specific disconnect reason.
func (c *Client) close(disconnect *Disconnect) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true

	if c.disconnect != nil {
		disconnect = c.disconnect
	}

	channels := c.channels
	c.mu.Unlock()

	if len(channels) > 0 {
		// Unsubscribe from all channels.
		for channel := range c.channels {
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

	if disconnect != nil && disconnect.Reason != "" {
		c.node.logger.log(newLogEntry(LogLevelDebug, "closing client connection", map[string]interface{}{"client": c.uid, "user": c.user, "reason": disconnect.Reason, "reconnect": disconnect.Reconnect}))
	}

	c.transport.Close(disconnect)

	if c.eventHub.disconnectHandler != nil {
		c.eventHub.disconnectHandler(DisconnectEvent{
			Disconnect: disconnect,
		})
	}

	return nil
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

// handle dispatches Command into correct command handler.
func (c *Client) handle(command *proto.Command) (*proto.Reply, *Disconnect) {

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, nil
	}
	c.mu.Unlock()

	var replyRes []byte
	var replyErr *proto.Error
	var disconnect *Disconnect

	method := command.Method
	params := command.Params

	if command.ID == 0 && method != proto.MethodTypeSend {
		c.node.logger.log(newLogEntry(LogLevelInfo, "command ID required for commands with reply expected", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
		replyErr = ErrorBadRequest
	} else if method != proto.MethodTypeConnect && !c.authenticated {
		// Client must send connect command first.
		replyErr = ErrorUnauthorized
	} else {
		started := time.Now()
		switch method {
		case proto.MethodTypeConnect:
			replyRes, replyErr, disconnect = c.handleConnect(params)
		case proto.MethodTypeRefresh:
			replyRes, replyErr, disconnect = c.handleRefresh(params)
		case proto.MethodTypeSubscribe:
			replyRes, replyErr, disconnect = c.handleSubscribe(params)
		case proto.MethodTypeSubRefresh:
			replyRes, replyErr, disconnect = c.handleSubRefresh(params)
		case proto.MethodTypeUnsubscribe:
			replyRes, replyErr, disconnect = c.handleUnsubscribe(params)
		case proto.MethodTypePublish:
			replyRes, replyErr, disconnect = c.handlePublish(params)
		case proto.MethodTypePresence:
			replyRes, replyErr, disconnect = c.handlePresence(params)
		case proto.MethodTypePresenceStats:
			replyRes, replyErr, disconnect = c.handlePresenceStats(params)
		case proto.MethodTypeHistory:
			replyRes, replyErr, disconnect = c.handleHistory(params)
		case proto.MethodTypePing:
			replyRes, replyErr, disconnect = c.handlePing(params)
		case proto.MethodTypeRPC:
			replyRes, replyErr, disconnect = c.handleRPC(params)
		case proto.MethodTypeSend:
			disconnect = c.handleSend(params)
		default:
			replyRes, replyErr = nil, ErrorMethodNotFound
		}
		commandDurationSummary.WithLabelValues(strings.ToLower(proto.MethodType_name[int32(method)])).Observe(time.Since(started).Seconds())
	}

	if disconnect != nil {
		return nil, disconnect
	}

	if command.ID == 0 {
		// Asynchronous message from client - no need to reply.
		return nil, nil
	}

	if replyErr != nil {
		replyErrorCount.WithLabelValues(strings.ToLower(proto.MethodType_name[int32(method)]), strconv.FormatUint(uint64(replyErr.Code), 10)).Inc()
	}

	rep := &proto.Reply{
		ID:    command.ID,
		Error: replyErr,
	}
	if replyRes != nil {
		rep.Result = replyRes
	}

	return rep, nil
}

func (c *Client) expire() {

	if c.eventHub.refreshHandler != nil {
		reply := c.eventHub.refreshHandler(RefreshEvent{})
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

	ttl := exp - time.Now().Unix()

	if c.eventHub.refreshHandler != nil {
		if ttl > 0 {
			c.mu.RLock()
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			c.mu.RUnlock()

			duration := time.Duration(ttl) * time.Second

			c.mu.Lock()
			c.expireTimer = time.AfterFunc(duration, c.expire)
			c.mu.Unlock()
		}
	}

	if ttl > 0 {
		// Connection was successfully refreshed.
		return
	}

	c.close(DisconnectExpired)
}

func (c *Client) handleConnect(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeConnect(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding connect", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.connectCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodeConnectResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding connect", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handleRefresh(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding refresh", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.refreshCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodeRefreshResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding refresh", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handleSubscribe(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeSubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding subscribe", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.subscribeCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodeSubscribeResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding subscribe", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handleSubRefresh(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeSubRefresh(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding sub refresh", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.subRefreshCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodeSubRefreshResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding sub refresh", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handleUnsubscribe(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeUnsubscribe(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding unsubscribe", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.unsubscribeCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodeUnsubscribeResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding unsubscribe", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handlePublish(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodePublish(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding publish", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.publishCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodePublishResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding publish", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handlePresence(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodePresence(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.presenceCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodePresenceResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handlePresenceStats(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodePresenceStats(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding presence stats", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.presenceStatsCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodePresenceStatsResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding presence stats", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handleHistory(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeHistory(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding history", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.historyCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodeHistoryResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding history", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handlePing(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodePing(params)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding ping", map[string]interface{}{"error": err.Error()}))
		return nil, nil, DisconnectBadRequest
	}
	resp, disconnect := c.pingCmd(cmd)
	if disconnect != nil {
		return nil, nil, disconnect
	}
	if resp.Error != nil {
		return nil, resp.Error, nil
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodePingResult(resp.Result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding ping", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *Client) handleRPC(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	if c.eventHub.rpcHandler != nil {
		cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeRPC(params)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding rpc", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectBadRequest
		}
		rpcReply := c.eventHub.rpcHandler(RPCEvent{
			Data: cmd.Data,
		})
		if rpcReply.Disconnect != nil {
			return nil, nil, rpcReply.Disconnect
		}
		if rpcReply.Error != nil {
			return nil, rpcReply.Error, nil
		}

		result := &proto.RPCResult{
			Data: rpcReply.Data,
		}

		var replyRes []byte
		replyRes, err = proto.GetResultEncoder(c.transport.Encoding()).EncodeRPCResult(result)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error encoding rpc", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectServerError
		}
		return replyRes, nil, nil
	}
	return nil, ErrorNotAvailable, nil
}

func (c *Client) handleSend(params proto.Raw) *Disconnect {
	if c.eventHub.messageHandler != nil {
		cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeSend(params)
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
	User       string    `json:"user"`
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
		return nil, nil
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
	if val := c.ctx.Value(credentialsContextKey); val != nil {
		if creds, ok := val.(*Credentials); ok {
			credentials = creds
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
		// Explicit auth Credentials not provided in context, try to look
		// for credentials in connect token.
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
			user = claims.User
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
	c.channels = make(map[string]ChannelContext)
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

	if c.node.eventHub.connectHandler != nil {
		reply := c.node.eventHub.connectHandler(c.ctx, c, ConnectEvent{
			Data: cmd.Data,
		})
		if reply.Disconnect != nil {
			return resp, reply.Disconnect
		}
		if reply.Error != nil {
			resp.Error = reply.Error
			return resp, nil
		}
		if reply.Data != nil {
			resp.Result.Data = reply.Data
		}
	}

	if c.eventHub.refreshHandler != nil {
		resp.Result.Expires = false
		resp.Result.TTL = 0
	}

	resp.Result.Client = c.uid

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
		user = claims.User
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
func (c *Client) subscribeCmd(cmd *proto.SubscribeRequest) (*proto.SubscribeResponse, *Disconnect) {

	channel := cmd.Channel
	if channel == "" {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel required for subscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &proto.SubscribeResponse{}

	config := c.node.Config()

	secret := config.Secret
	channelMaxLength := config.ChannelMaxLength
	channelLimit := config.ClientChannelLimit
	insecure := config.ClientInsecure

	res := &proto.SubscribeResult{}

	if channelMaxLength > 0 && len(channel) > channelMaxLength {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel too long", map[string]interface{}{"max": channelMaxLength, "channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = ErrorLimitExceeded
		return resp, nil
	}

	c.mu.RLock()
	numChannels := len(c.channels)
	c.mu.RUnlock()

	if channelLimit > 0 && numChannels >= channelLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "maximum limit of channels per client reached", map[string]interface{}{"limit": channelLimit, "user": c.user, "client": c.uid}))
		resp.Error = ErrorLimitExceeded
		return resp, nil
	}

	c.mu.RLock()
	_, ok := c.channels[channel]
	c.mu.RUnlock()

	if ok {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already subscribed on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = ErrorAlreadySubscribed
		return resp, nil
	}

	if !c.node.userAllowed(channel, c.user) {
		c.node.logger.log(newLogEntry(LogLevelInfo, "user is not allowed to subscribe on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	if !chOpts.Anonymous && c.user == "" && !insecure {
		c.node.logger.log(newLogEntry(LogLevelInfo, "anonymous user is not allowed to subscribe on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = ErrorPermissionDenied
		return resp, nil
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
			resp.Error = ErrorPermissionDenied
			return resp, nil
		}
		if claims, ok := parsedToken.Claims.(*subscribeTokenClaims); ok && parsedToken.Valid {
			tokenChannel = claims.Channel
			tokenInfo = claims.Info
			tokenB64info = claims.Base64Info
			tokenClient = claims.Client
			expireAt = claims.StandardClaims.ExpiresAt

			if c.uid != tokenClient {
				resp.Error = ErrorPermissionDenied
				return resp, nil
			}
			if cmd.Channel != tokenChannel {
				resp.Error = ErrorPermissionDenied
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
		} else {
			if validationErr, ok := err.(*jwt.ValidationError); ok {
				if validationErr.Errors == jwt.ValidationErrorExpired {
					// The only problem with token is its expiration - no other errors set in bitfield.
					resp.Error = ErrorTokenExpired
					return resp, nil
				}
				c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
				resp.Error = ErrorPermissionDenied
				return resp, nil
			}
			c.node.logger.log(newLogEntry(LogLevelInfo, "invalid subscription token", map[string]interface{}{"error": err.Error(), "client": c.uid, "user": c.UserID()}))
			resp.Error = ErrorPermissionDenied
			return resp, nil
		}
	}

	if c.eventHub.subscribeHandler != nil {
		reply := c.eventHub.subscribeHandler(SubscribeEvent{
			Channel: channel,
		})
		if reply.Disconnect != nil {
			return resp, reply.Disconnect
		}
		if reply.Error != nil {
			resp.Error = reply.Error
			return resp, nil
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
			resp.Error = ErrorExpired
			return resp, nil
		}
		if isPrivateChannel {
			// Only expose expiration info to client in private channel case.
			// In other scenarios expiration will be handled by SubRefreshHandler
			// on Go application backend side.
			res.Expires = true
			res.TTL = uint32(expireAt - now)
		}
	}

	channelContext := ChannelContext{
		Info:     channelInfo,
		expireAt: expireAt,
	}
	c.mu.Lock()
	c.channels[channel] = channelContext
	c.mu.Unlock()

	err := c.node.addSubscription(channel, c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		return nil, DisconnectServerError
	}

	c.mu.RLock()
	info := c.clientInfo(channel)
	c.mu.RUnlock()

	if chOpts.Presence {
		err = c.node.addPresence(channel, c.uid, info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error adding presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			return nil, DisconnectServerError
		}
	}

	if chOpts.HistoryRecover {
		if cmd.Recover {
			// Client provided subscribe request with recover flag on. Try to recover missed
			// publications automatically from history (we suppose here that history configured wisely)
			// based on provided last publication uid seen by client.
			if cmd.Last == "" {
				// Client wants to recover publications but it seems that there were no
				// messages in channel history before, so looks like client missed all
				// existing messages. Though in this case we can't guarantee that messages
				// were fully recovered.
				publications, err := c.node.History(channel)
				if err != nil {
					c.node.logger.log(newLogEntry(LogLevelError, "error recovering", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
					res.Publications = nil
					res.Recovered = false
				} else {
					res.Publications = publications
					res.Recovered = time.Duration(cmd.Away)*time.Second+time.Second < time.Duration(chOpts.HistoryLifetime)*time.Second && len(publications) < chOpts.HistorySize
				}
			} else {
				publications, found, err := c.node.recoverHistory(channel, cmd.Last)
				if err != nil {
					c.node.logger.log(newLogEntry(LogLevelError, "error recovering", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
					res.Publications = nil
					res.Recovered = false
				} else {
					res.Publications = publications
					res.Recovered = found || (time.Duration(cmd.Away)*time.Second+time.Second < time.Duration(chOpts.HistoryLifetime)*time.Second && len(publications) < chOpts.HistorySize)
				}
			}
		} else {
			// Client don't want to recover messages yet (fresh connect), we just return last
			// publication uid here so it could recover later.
			lastPubUID, err := c.node.lastPublicationUID(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error getting last publication ID for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			} else {
				res.Last = lastPubUID
			}
		}
	}

	if chOpts.JoinLeave {
		join := &proto.Join{
			Info: *info,
		}
		go c.node.publishJoin(channel, join, &chOpts)
	}
	resp.Result = res
	return resp, nil
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

	channelContext := ChannelContext{
		Info:     channelInfo,
		expireAt: expireAt,
	}
	c.mu.Lock()
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

	pub := &Publication{
		Data: data,
		Info: info,
	}

	if c.eventHub.publishHandler != nil {
		reply := c.eventHub.publishHandler(PublishEvent{
			Channel:     ch,
			Publication: pub,
		})
		if reply.Disconnect != nil {
			return resp, reply.Disconnect
		}
		if reply.Error != nil {
			resp.Error = reply.Error
			return resp, nil
		}
	}

	err := <-c.node.publish(ch, pub, &chOpts)
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

	if !chOpts.Presence {
		resp.Error = ErrorNotAvailable
		return resp, nil
	}

	presence, err := c.node.Presence(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error getting presence", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
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

	if !chOpts.Presence {
		resp.Error = ErrorNotAvailable
		return resp, nil
	}

	stats, err := c.node.presenceStats(ch)
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

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
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
	var res *proto.PingResult
	if cmd.Data != "" {
		res = &proto.PingResult{
			Data: cmd.Data,
		}
	}

	resp := &proto.PingResponse{
		Result: res,
	}

	return resp, nil
}
