package centrifuge

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/auth"
	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/uuid"
)

// ClientEventHub allows to deal with client event handlers.
// All its methods are not goroutine-safe and supposed to be called once on client connect.
type ClientEventHub struct {
	disconnectHandler  DisconnectHandler
	subscribeHandler   SubscribeHandler
	unsubscribeHandler UnsubscribeHandler
	publishHandler     PublishHandler
	refreshHandler     RefreshHandler
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
	UserID string
	Exp    int64
	Info   []byte
}

// credentialsContextKeyType ...
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
	Info proto.Raw
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

// newClient creates new client connection.
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
// At moment used to close connections which have not sent valid connect command
// in a reasonable time interval after actually connected to Centrifugo.
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

// updatePresence updates presence info for all client channels
func (c *Client) updatePresence() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	for channel := range c.channels {
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

// User return user ID associated with client connection.
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
	result, err := pushEncoder.Encode(proto.NewMessage(data))
	if err != nil {
		return err
	}

	reply := newPreparedReply(&proto.Reply{
		Result: result,
	}, c.transport.Encoding())

	return c.transport.Send(reply)
}

// Unsubscribe allows to unsubscribe client from channel.
func (c *Client) Unsubscribe(ch string) error {
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
	c.sendUnsub(ch)
	return nil
}

func (c *Client) sendUnsub(ch string) error {
	pushEncoder := proto.GetPushEncoder(c.transport.Encoding())

	data, err := pushEncoder.EncodeUnsub(&proto.Unsub{})
	if err != nil {
		return err
	}
	result, err := pushEncoder.Encode(proto.NewUnsub(ch, data))
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
		// unsubscribe from all channels
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
		replyErrorCount.WithLabelValues(strings.ToLower(proto.MethodType_name[int32(method)])).Inc()
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
	config := c.node.Config()
	clientExpire := config.ClientExpire

	if !clientExpire {
		return
	}

	if c.eventHub.refreshHandler != nil {
		reply := c.eventHub.refreshHandler(RefreshEvent{})
		if reply.Exp > 0 {
			c.exp = reply.Exp
			if reply.Info != nil {
				c.info = reply.Info
			}
		}
	}

	c.mu.RLock()
	exp := c.exp
	c.mu.RUnlock()

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

// connectCmd handles connect command from client - client must send connect
// command immediately after establishing connection with Centrifugo.
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
	clientExpire := config.ClientExpire
	closeDelay := config.ClientExpiredCloseDelay
	userConnectionLimit := config.ClientUserConnectionLimit

	var credentials *Credentials
	if val := c.ctx.Value(credentialsContextKey); val != nil {
		if creds, ok := val.(*Credentials); ok {
			credentials = creds
		}
	}

	var expires bool
	var expired bool
	var ttl uint32

	if credentials != nil {
		c.mu.Lock()
		c.user = credentials.UserID
		c.info = credentials.Info
		c.exp = credentials.Exp
		c.mu.Unlock()
	} else {
		// explicit authentication Credentials not provided in middlewares, look
		// for signed credentials in connect command.
		signedCredentials := cmd.Credentials
		if signedCredentials != nil {
			user := signedCredentials.User
			info := signedCredentials.Info
			opts := signedCredentials.Opts

			var exp string
			var sign string
			if !insecure {
				exp = signedCredentials.Exp
				sign = signedCredentials.Sign
			}
			if !insecure {
				secret := config.Secret
				isValid := auth.CheckClientSign(secret, user, exp, info, opts, sign)
				if !isValid {
					c.node.logger.log(newLogEntry(LogLevelInfo, "invalid sign", map[string]interface{}{"user": user, "client": c.uid}))
					return resp, DisconnectInvalidSign
				}
				timestamp, err := strconv.Atoi(exp)
				if err != nil {
					c.node.logger.log(newLogEntry(LogLevelInfo, "invalid exp timestamp", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
					return resp, DisconnectBadRequest
				}
				c.mu.Lock()
				c.exp = int64(timestamp)
				c.mu.Unlock()
			}

			c.mu.Lock()
			c.user = user
			c.mu.Unlock()

			if len(info) > 0 {
				byteInfo, err := base64.StdEncoding.DecodeString(info)
				if err != nil {
					c.node.logger.log(newLogEntry(LogLevelInfo, "can not decode provided info from base64", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
					return resp, DisconnectBadRequest
				}
				c.mu.Lock()
				c.info = proto.Raw(byteInfo)
				c.mu.Unlock()
			}
		} else {
			if !insecure {
				c.node.logger.log(newLogEntry(LogLevelInfo, "client credentials not provided in connect", map[string]interface{}{"client": c.uid, "user": c.user}))
				return resp, DisconnectBadRequest
			}
		}
	}

	c.mu.RLock()
	user := c.user
	c.mu.RUnlock()

	c.node.logger.log(newLogEntry(LogLevelDebug, "client authenticated", map[string]interface{}{"client": c.uid, "user": c.user}))

	if userConnectionLimit > 0 && user != "" && len(c.node.hub.userConnections(user)) >= userConnectionLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "limit of connections for user reached", map[string]interface{}{"user": user, "client": c.uid, "limit": userConnectionLimit}))
		resp.Error = ErrorLimitExceeded
		return resp, nil
	}

	if clientExpire && !insecure {
		expires = true
		c.mu.RLock()
		diff := c.exp - time.Now().Unix()
		c.mu.RUnlock()
		if diff <= 0 {
			expired = true
			ttl = 0
		} else {
			ttl = uint32(diff)
		}
	}

	res := &proto.ConnectResult{
		Version: version,
		Expires: expires,
		TTL:     ttl,
		Expired: expired,
	}

	resp.Result = res

	if expired {
		if credentials != nil {
			// In case of native Go authentication disconnect user.
			return nil, DisconnectExpired
		}
		// In case of using signed credentials initiate refresh workflow.
		return resp, nil
	}

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

	if clientExpire && !expired {
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

	signedCredentials := cmd.Credentials
	if signedCredentials == nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client credentials not provided in refresh", map[string]interface{}{"user": c.user, "client": c.uid}))
		return resp, DisconnectBadRequest
	}

	user := signedCredentials.User
	exp := signedCredentials.Exp
	info := signedCredentials.Info
	opts := signedCredentials.Opts
	sign := signedCredentials.Sign

	config := c.node.Config()
	secret := config.Secret

	isValid := auth.CheckClientSign(secret, user, exp, info, opts, sign)
	if !isValid {
		c.node.logger.log(newLogEntry(LogLevelInfo, "invalid refresh sign", map[string]interface{}{"user": user, "client": c.uid}))
		return resp, DisconnectInvalidSign
	}

	var byteInfo []byte
	if len(info) > 0 {
		var err error
		byteInfo, err = base64.StdEncoding.DecodeString(info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "can not decode provided info from base64", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
			return resp, DisconnectBadRequest
		}
	}

	timestamp, err := strconv.Atoi(exp)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelInfo, "invalid refresh exp timestamp", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
		return resp, DisconnectBadRequest
	}

	closeDelay := config.ClientExpiredCloseDelay
	clientExpire := config.ClientExpire
	version := config.Version

	res := &proto.RefreshResult{
		Version: version,
		Expires: clientExpire,
		Client:  c.uid,
	}

	diff := int64(timestamp) - time.Now().Unix()
	if diff > 0 {
		res.TTL = uint32(diff)
	}

	resp.Result = res

	if clientExpire {
		// connection check enabled
		timeToExpire := int64(timestamp) - time.Now().Unix()
		if timeToExpire > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.mu.Lock()
			c.exp = int64(timestamp)
			if len(byteInfo) > 0 {
				c.info = proto.Raw(byteInfo)
			}
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(timeToExpire)*time.Second + closeDelay
			c.expireTimer = time.AfterFunc(duration, c.expire)
			c.mu.Unlock()
		} else {
			resp.Result.Expired = true
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
	if c.node.privateChannel(channel) {
		// private channel - subscription must be properly signed
		if c.uid != cmd.Client {
			resp.Error = ErrorPermissionDenied
			return resp, nil
		}
		isValid := auth.CheckChannelSign(secret, cmd.Client, channel, cmd.Info, cmd.Sign)
		if !isValid {
			resp.Error = ErrorPermissionDenied
			return resp, nil
		}
		if len(cmd.Info) > 0 {
			channelInfo = proto.Raw(cmd.Info)
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
		if len(reply.ChannelInfo) > 0 {
			channelInfo = reply.ChannelInfo
		}
	}

	channelContext := ChannelContext{
		Info: channelInfo,
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
			// Client provided subscribe request with recover flag on. Try to recover missed publications
			// automatically from history (we suppose here that history configured wisely) based on
			// provided last publication uid seen by client.
			publications, recovered, err := c.node.recoverHistory(channel, cmd.Last)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error recovering publications", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				res.Publications = nil
			} else {
				res.Publications = publications
				res.Recovered = recovered
			}
		} else {
			// Client don't want to recover messages yet (fresh connect), we just return last message id here so it could recover later.
			lastPubUID, err := c.node.lastPublicationUID(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error getting last message ID for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
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

	pub := &proto.Publication{
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

// presenceStatsCmd handle request to get presence stats – short summary
// about clients in channel.
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
