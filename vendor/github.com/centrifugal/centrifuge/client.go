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

	uuid "github.com/satori/go.uuid"
)

// Client interface contains functions to inspect and control client
// connection.
type Client interface {
	// ID returns unique client connection id.
	ID() string
	// User return user ID associated with client connection.
	UserID() string
	// Channels returns a slice of channels client connection subscribed to at moment.
	Channels() []string
	// Transport returns transport used by client connection.
	Transport() Transport
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
var CredentialsContextKey credentialsContextKeyType

// client represents client connection to Centrifugo. Transport of
// incoming connection abstracted away via Transport interface.
type client struct {
	mu            sync.RWMutex
	ctx           context.Context
	node          *Node
	transport     transport
	authenticated bool
	uid           string
	user          string
	exp           int64
	info          proto.Raw
	channelInfo   map[string]proto.Raw
	channels      map[string]struct{}
	closed        bool
	staleTimer    *time.Timer
	expireTimer   *time.Timer
	presenceTimer *time.Timer
}

// newClient creates new client connection.
func newClient(ctx context.Context, n *Node, t transport) *client {
	c := &client{
		ctx:       ctx,
		uid:       uuid.NewV4().String(),
		node:      n,
		transport: t,
	}

	config := n.Config()
	staleCloseDelay := config.ClientStaleCloseDelay
	if staleCloseDelay > 0 && !c.authenticated {
		c.mu.Lock()
		c.staleTimer = time.AfterFunc(staleCloseDelay, c.closeUnauthenticated)
		c.mu.Unlock()
	}

	return c
}

// closeUnauthenticated closes connection if it's not authenticated yet.
// At moment used to close connections which have not sent valid connect command
// in a reasonable time interval after actually connected to Centrifugo.
func (c *client) closeUnauthenticated() {
	c.mu.RLock()
	authenticated := c.authenticated
	closed := c.closed
	c.mu.RUnlock()
	if !authenticated && !closed {
		c.Close(&Disconnect{Reason: "stale", Reconnect: false})
	}
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect
func (c *client) updateChannelPresence(ch string) error {
	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		return ErrorNamespaceNotFound
	}
	if !chOpts.Presence {
		return nil
	}
	return c.node.addPresence(ch, c.uid, c.clientInfo(ch))
}

// updatePresence updates presence info for all client channels
func (c *client) updatePresence() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	channels := make([]string, len(c.channels))
	i := 0
	for k := range c.channels {
		channels[i] = k
		i++
	}
	if c.node.Mediator() != nil && c.node.Mediator().Presence != nil {
		c.node.Mediator().Presence(c.ctx, PresenceContext{
			EventContext: EventContext{
				Client: c,
			},
			Channels: channels,
		})
	}
	for _, channel := range channels {
		err := c.updateChannelPresence(channel)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error updating presence for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		}
	}
	c.addPresenceUpdate()
}

// Lock must be held outside.
func (c *client) addPresenceUpdate() {
	config := c.node.Config()
	presenceInterval := config.ClientPresencePingInterval
	c.presenceTimer = time.AfterFunc(presenceInterval, c.updatePresence)
}

// No lock here as uid set on client initialization and can not be changed - we
// only read this value after.
func (c *client) ID() string {
	return c.uid
}

// UserID returns ID of user. No locking here as User() can not be called before
// we set user value in connect command - after this we only read this value.
func (c *client) UserID() string {
	return c.user
}

func (c *client) Transport() Transport {
	return c.transport
}

func (c *client) Channels() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, len(c.channels))
	i := 0
	for k := range c.channels {
		keys[i] = k
		i++
	}
	return keys
}

func (c *client) Unsubscribe(ch string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	err := c.unsubscribe(ch)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()
	c.sendUnsubscribe(ch)
	return nil
}

func (c *client) sendUnsubscribe(ch string) error {
	var messageEncoder proto.MessageEncoder
	if c.transport.Encoding() == proto.EncodingJSON {
		messageEncoder = proto.NewJSONMessageEncoder()
	} else {
		messageEncoder = proto.NewProtobufMessageEncoder()
	}

	data, err := messageEncoder.EncodeUnsub(&proto.Unsub{})
	if err != nil {
		return err
	}
	result, err := messageEncoder.Encode(proto.NewUnsubMessage(ch, data))
	if err != nil {
		return nil
	}

	reply := newPreparedReply(&proto.Reply{
		Result: result,
	}, c.transport.Encoding())

	c.transport.Send(reply)

	return nil
}

// Close client connection with specific disconnect reason.
func (c *client) Close(disconnect *Disconnect) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if len(c.channels) > 0 {
		// unsubscribe from all channels
		for channel := range c.channels {
			err := c.unsubscribe(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error unsubscribing client from channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			}
		}
	}

	if c.authenticated {
		err := c.node.removeClient(c)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error removing client", map[string]interface{}{"user": c.user, "client": c.uid, "error": err.Error()}))
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

	if disconnect != nil && disconnect.Reason != "" {
		c.node.logger.log(newLogEntry(LogLevelDebug, "closing client connection", map[string]interface{}{"client": c.uid, "user": c.user, "reason": disconnect.Reason, "reconnect": disconnect.Reconnect}))
	}

	c.transport.Close(disconnect)

	if c.node.Mediator() != nil && c.node.Mediator().Disconnect != nil {
		c.node.Mediator().Disconnect(c.ctx, DisconnectContext{
			EventContext: EventContext{
				Client: c,
			},
			Disconnect: disconnect,
		})
	}

	return nil
}

func (c *client) clientInfo(ch string) *proto.ClientInfo {
	channelInfo, _ := c.channelInfo[ch]
	return &proto.ClientInfo{
		User:     c.user,
		Client:   c.uid,
		ConnInfo: c.info,
		ChanInfo: channelInfo,
	}
}

// handle dispatches Command into correct command handler.
func (c *client) handle(command *proto.Command) (*proto.Reply, *Disconnect) {

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, nil
	}

	var replyRes []byte
	var replyErr *proto.Error
	var disconnect *Disconnect

	method := command.Method
	params := command.Params

	if command.ID == 0 && method != proto.MethodTypeMessage {
		c.node.logger.log(newLogEntry(LogLevelInfo, "command ID required for synchronous messages", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
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
		case proto.MethodTypeMessage:
			disconnect = c.handleMessage(params)
		default:
			replyRes, replyErr = nil, ErrorMethodNotFound
		}
		commandDurationSummary.WithLabelValues(strings.ToLower(proto.MethodType_name[int32(method)])).Observe(float64(time.Since(started).Seconds()))
	}

	if disconnect != nil {
		return nil, disconnect
	}

	if command.ID == 0 {
		// Asynchronous message from client - no need to reply.
		return nil, nil
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

func (c *client) expire() {
	config := c.node.Config()
	clientExpire := config.ClientExpire

	if !clientExpire {
		return
	}

	mediator := c.node.Mediator()
	if mediator != nil && mediator.Refresh != nil {
		reply := mediator.Refresh(c.ctx, RefreshContext{
			EventContext: EventContext{
				Client: c,
			},
		})
		if reply.Exp > 0 {
			c.exp = reply.Exp
			if reply.Info != nil {
				c.info = reply.Info
			}
		}
	}

	c.mu.RLock()
	ttl := c.exp - time.Now().Unix()
	c.mu.RUnlock()

	if mediator != nil && mediator.Refresh != nil {
		if ttl > 0 {
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(ttl) * time.Second
			c.expireTimer = time.AfterFunc(duration, c.expire)
		}
	}

	if ttl > 0 {
		// connection was successfully refreshed.
		return
	}

	c.Close(&Disconnect{Reason: "expired", Reconnect: true})
	return
}

func (c *client) handleConnect(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handleRefresh(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handleSubscribe(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handleUnsubscribe(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handlePublish(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handlePresence(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handlePresenceStats(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handleHistory(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handlePing(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
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

func (c *client) handleRPC(params proto.Raw) (proto.Raw, *proto.Error, *Disconnect) {
	mediator := c.node.Mediator()
	if mediator != nil && mediator.RPC != nil {
		cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeRPC(params)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding rpc", map[string]interface{}{"error": err.Error()}))
			return nil, nil, DisconnectBadRequest
		}
		rpcReply := mediator.RPC(c.ctx, RPCContext{
			EventContext: EventContext{
				Client: c,
			},
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

func (c *client) handleMessage(params proto.Raw) *Disconnect {
	mediator := c.node.Mediator()
	if mediator != nil && mediator.Message != nil {
		cmd, err := proto.GetParamsDecoder(c.transport.Encoding()).DecodeMessage(params)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelInfo, "error decoding message", map[string]interface{}{"error": err.Error()}))
			return DisconnectBadRequest
		}
		messageReply := mediator.Message(c.ctx, MessageContext{
			EventContext: EventContext{
				Client: c,
			},
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
func (c *client) connectCmd(cmd *proto.ConnectRequest) (*proto.ConnectResponse, *Disconnect) {
	resp := &proto.ConnectResponse{}

	if c.authenticated {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already authenticated", map[string]interface{}{"client": c.uid, "user": c.user}))
		return resp, DisconnectBadRequest
	}

	config := c.node.Config()

	secret := config.Secret
	insecure := config.ClientInsecure
	closeDelay := config.ClientExpiredCloseDelay
	clientExpire := config.ClientExpire
	version := config.Version
	userConnectionLimit := config.ClientUserConnectionLimit

	var credentials *Credentials
	if val := c.ctx.Value(CredentialsContextKey); val != nil {
		if creds, ok := val.(*Credentials); ok {
			credentials = creds
		}
	}

	var expires bool
	var expired bool
	var ttl uint32

	if credentials != nil {
		c.user = credentials.UserID
		c.info = credentials.Info
		c.exp = credentials.Exp
		if c.exp > 0 {
			ttl = uint32(c.exp - time.Now().Unix())
		}
	} else {
		user := cmd.User
		info := cmd.Info
		opts := cmd.Opts

		var exp string
		var sign string
		if !insecure {
			exp = cmd.Exp
			sign = cmd.Sign
		}

		if !insecure {
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
			c.exp = int64(timestamp)
		}

		c.user = user
		if len(info) > 0 {
			byteInfo, err := base64.StdEncoding.DecodeString(info)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelInfo, "can not decode provided info from base64", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
				return resp, DisconnectBadRequest
			}
			c.info = proto.Raw(byteInfo)
		}
	}

	if userConnectionLimit > 0 && c.user != "" && len(c.node.hub.userConnections(c.user)) >= userConnectionLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "limit of connections for user reached", map[string]interface{}{"user": c.user, "client": c.uid, "limit": userConnectionLimit}))
		resp.Error = ErrorLimitExceeded
		return resp, nil
	}

	if clientExpire && !insecure {
		expires = true
		ttl = uint32(c.exp - time.Now().Unix())
		if ttl <= 0 {
			expired = true
			ttl = 0
		}
	}

	res := &proto.ConnectResult{
		Version: version,
		Expires: expires,
		TTL:     ttl,
		Expired: expired,
	}

	if c.node.Mediator() != nil && c.node.Mediator().Refresh != nil {
		res.Expires = false
		res.TTL = 0
	}

	resp.Result = res

	if expired {
		// Can't authenticate client with expired credentials.
		return resp, nil
	}

	// Client successfully connected.
	c.authenticated = true
	c.channels = map[string]struct{}{}
	c.channelInfo = map[string]proto.Raw{}

	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}

	c.addPresenceUpdate()

	err := c.node.addClient(c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding client", map[string]interface{}{"client": c.uid, "error": err.Error()}))
		return resp, DisconnectServerError
	}

	if clientExpire && !expired {
		duration := closeDelay + time.Duration(ttl)*time.Second
		c.expireTimer = time.AfterFunc(duration, c.expire)
	}

	resp.Result.Client = c.uid

	// TODO: check locking.
	if c.node.Mediator() != nil && c.node.Mediator().Connect != nil {
		c.node.Mediator().Connect(c.ctx, ConnectContext{
			EventContext: EventContext{
				Client: c,
			},
		})
	}

	return resp, nil
}

// refreshCmd handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime option set.
func (c *client) refreshCmd(cmd *proto.RefreshRequest) (*proto.RefreshResponse, *Disconnect) {

	resp := &proto.RefreshResponse{}

	user := cmd.User
	info := cmd.Info
	exp := cmd.Exp
	opts := cmd.Opts
	sign := cmd.Sign

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
		TTL:     uint32(int64(timestamp) - time.Now().Unix()),
		Client:  c.uid,
	}

	resp.Result = res

	if clientExpire {
		// connection check enabled
		timeToExpire := int64(timestamp) - time.Now().Unix()
		if timeToExpire > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.exp = int64(timestamp)
			if len(byteInfo) > 0 {
				c.info = proto.Raw(byteInfo)
			}
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(timeToExpire)*time.Second + closeDelay
			c.expireTimer = time.AfterFunc(duration, c.expire)
		} else {
			resp.Result.Expired = true
		}
	}
	return resp, nil
}

// NOTE: actually can be a part of engine history method, for
// example if we eventually will work with Redis streams. For
// this sth like HistoryFilter{limit int, from string} struct
// can be added as History engine method argument.
func recoverMessages(last string, messages []*proto.Publication) ([]*proto.Publication, bool) {
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
func (c *client) subscribeCmd(cmd *proto.SubscribeRequest) (*proto.SubscribeResponse, *Disconnect) {

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

	if len(channel) > channelMaxLength {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel too long", map[string]interface{}{"max": channelMaxLength, "channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = ErrorLimitExceeded
		return resp, nil
	}

	if len(c.channels) >= channelLimit {
		c.node.logger.log(newLogEntry(LogLevelInfo, "maximum limit of channels per client reached", map[string]interface{}{"limit": channelLimit, "user": c.user, "client": c.uid}))
		resp.Error = ErrorLimitExceeded
		return resp, nil
	}

	if _, ok := c.channels[channel]; ok {
		c.node.logger.log(newLogEntry(LogLevelInfo, "client already subscribed on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = ErrorAlreadySubscribed
		return resp, nil
	}

	if !c.node.userAllowed(channel, c.user) || !c.node.clientAllowed(channel, c.uid) {
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

	if c.node.privateChannel(channel) {
		// private channel - subscription must be properly signed
		if string(c.uid) != string(cmd.Client) {
			resp.Error = ErrorPermissionDenied
			return resp, nil
		}
		isValid := auth.CheckChannelSign(secret, string(cmd.Client), string(channel), cmd.Info, cmd.Sign)
		if !isValid {
			resp.Error = ErrorPermissionDenied
			return resp, nil
		}
		if len(cmd.Info) > 0 {
			c.channelInfo[channel] = proto.Raw(cmd.Info)
		}
	}

	if c.node.Mediator() != nil && c.node.Mediator().Subscribe != nil {
		reply := c.node.Mediator().Subscribe(c.ctx, SubscribeContext{
			EventContext: EventContext{
				Client: c,
			},
			Channel: channel,
		})
		if reply.Disconnect != nil {
			return resp, reply.Disconnect
		}
		if reply.Error != nil {
			resp.Error = reply.Error
			return resp, nil
		}
	}

	c.channels[channel] = struct{}{}

	err := c.node.addSubscription(channel, c)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error adding subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		return nil, DisconnectServerError
	}

	info := c.clientInfo(channel)

	if chOpts.Presence {
		err = c.node.addPresence(channel, c.uid, info)
		if err != nil {
			c.node.logger.log(newLogEntry(LogLevelError, "error adding presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			return nil, DisconnectServerError
		}
	}

	if chOpts.HistoryRecover {
		if cmd.Recover {
			// Client provided subscribe request with recover flag on. Try to recover missed messages
			// automatically from history (we suppose here that history configured wisely) based on
			// provided last message id value.
			messages, err := c.node.History(channel)
			if err != nil {
				c.node.logger.log(newLogEntry(LogLevelError, "error recovering messages", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				res.Publications = nil
			} else {
				recoveredMessages, recovered := recoverMessages(cmd.Last, messages)
				res.Publications = recoveredMessages
				res.Recovered = recovered
			}
		} else {
			// Client don't want to recover messages yet, we just return last message id to him here.
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

func (c *client) unsubscribe(channel string) error {
	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		return ErrorNamespaceNotFound
	}

	info := c.clientInfo(channel)

	_, ok = c.channels[channel]
	if ok {
		delete(c.channels, channel)

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
	}

	if c.node.Mediator() != nil && c.node.Mediator().Unsubscribe != nil {
		c.node.Mediator().Unsubscribe(c.ctx, UnsubscribeContext{
			EventContext: EventContext{
				Client: c,
			},
			Channel: channel,
		})
	}

	return nil
}

// unsubscribeCmd handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *client) unsubscribeCmd(cmd *proto.UnsubscribeRequest) (*proto.UnsubscribeResponse, *Disconnect) {

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
func (c *client) publishCmd(cmd *proto.PublishRequest) (*proto.PublishResponse, *Disconnect) {

	ch := cmd.Channel
	data := cmd.Data

	if string(ch) == "" || len(data) == 0 {
		c.node.logger.log(newLogEntry(LogLevelInfo, "channel and data required for publish", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, DisconnectBadRequest
	}

	resp := &proto.PublishResponse{}

	if _, ok := c.channels[ch]; !ok {
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	info := c.clientInfo(ch)

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		c.node.logger.log(newLogEntry(LogLevelInfo, "attempt to subscribe on non-existing namespace", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid}))
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	insecure := c.node.Config().ClientInsecure

	if !chOpts.Publish && !insecure {
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	publication := &proto.Publication{
		Data: data,
		Info: info,
	}

	if c.node.Mediator() != nil && c.node.Mediator().Publish != nil {
		reply := c.node.Mediator().Publish(c.ctx, PublishContext{
			EventContext: EventContext{
				Client: c,
			},
			Channel:     ch,
			Publication: publication,
		})
		if reply.Disconnect != nil {
			return resp, reply.Disconnect
		}
		if reply.Error != nil {
			resp.Error = reply.Error
			return resp, nil
		}
	}

	err := <-c.node.publish(ch, publication, &chOpts)
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
func (c *client) presenceCmd(cmd *proto.PresenceRequest) (*proto.PresenceResponse, *Disconnect) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, DisconnectBadRequest
	}

	resp := &proto.PresenceResponse{}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	if _, ok := c.channels[ch]; !ok {
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

// presenceStatsCmd ...
func (c *client) presenceStatsCmd(cmd *proto.PresenceStatsRequest) (*proto.PresenceStatsResponse, *Disconnect) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, DisconnectBadRequest
	}

	resp := &proto.PresenceStatsResponse{}

	if _, ok := c.channels[ch]; !ok {
		resp.Error = ErrorPermissionDenied
		return resp, nil
	}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = ErrorNamespaceNotFound
		return resp, nil
	}

	if !chOpts.Presence || !chOpts.PresenceStats {
		resp.Error = ErrorNotAvailable
		return resp, nil
	}

	presence, err := c.node.Presence(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error getting presence", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp, nil
	}

	numClients := len(presence)
	numUsers := 0
	uniqueUsers := map[string]struct{}{}

	for _, info := range presence {
		userID := info.User
		if _, ok := uniqueUsers[userID]; !ok {
			uniqueUsers[userID] = struct{}{}
			numUsers++
		}
	}

	resp.Result = &proto.PresenceStatsResult{
		NumClients: uint32(numClients),
		NumUsers:   uint32(numUsers),
	}

	return resp, nil
}

// historyCmd handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag).
func (c *client) historyCmd(cmd *proto.HistoryRequest) (*proto.HistoryResponse, *Disconnect) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, DisconnectBadRequest
	}

	resp := &proto.HistoryResponse{}

	if _, ok := c.channels[ch]; !ok {
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

	publications, err := c.node.History(ch)
	if err != nil {
		c.node.logger.log(newLogEntry(LogLevelError, "error getting history", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = ErrorInternal
		return resp, nil
	}

	resp.Result = &proto.HistoryResult{
		Publications: publications,
	}

	return resp, nil
}

// pingCmd handles ping command from client.
func (c *client) pingCmd(cmd *proto.PingRequest) (*proto.PingResponse, *Disconnect) {
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
