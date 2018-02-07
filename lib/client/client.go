package client

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/lib/auth"
	"github.com/centrifugal/centrifugo/lib/conns"
	"github.com/centrifugal/centrifugo/lib/events"
	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"

	"github.com/satori/go.uuid"
)

func init() {
	// TODO: move to transport?
	// metricsRegistry.RegisterCounter("client_num_msg_queued", metrics.NewCounter())
	// metricsRegistry.RegisterCounter("client_num_msg_sent", metrics.NewCounter())
}

// Client represents client connection to Centrifugo - at moment
// this can be Websocket or SockJS connection. Transport of incoming
// connection abstracted away via Session interface.
type client struct {
	mu            sync.RWMutex
	ctx           context.Context
	node          *node.Node
	transport     conns.Transport
	authenticated bool
	uid           string
	user          string
	exp           int64
	opts          string
	connInfo      proto.Raw
	channelInfo   map[string]proto.Raw
	channels      map[string]struct{}
	closed        bool
	staleTimer    *time.Timer
	expireTimer   *time.Timer
	presenceTimer *time.Timer
	encoding      proto.Encoding
}

// New creates new client connection.
func New(ctx context.Context, n *node.Node, t conns.Transport, conf Config) conns.Client {
	c := &client{
		ctx:       ctx,
		uid:       uuid.NewV4().String(),
		node:      n,
		transport: t,
		encoding:  conf.Encoding,
	}

	config := n.Config()
	staleCloseDelay := config.ClientStaleCloseDelay
	if staleCloseDelay > 0 && !c.authenticated {
		c.staleTimer = time.AfterFunc(staleCloseDelay, c.closeUnauthenticated)
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
		c.Close(&proto.Disconnect{Reason: "stale", Reconnect: false})
	}
}

// updateChannelPresence updates client presence info for channel so it
// won't expire until client disconnect
func (c *client) updateChannelPresence(ch string) {
	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		return
	}
	if !chOpts.Presence {
		return
	}
	c.node.AddPresence(ch, c.uid, c.info(ch))
}

// updatePresence updates presence info for all client channels
func (c *client) updatePresence() {
	c.mu.RLock()
	if c.closed {
		return
	}
	for _, channel := range c.Channels() {
		c.updateChannelPresence(channel)
	}
	c.mu.RUnlock()
	c.mu.Lock()
	c.addPresenceUpdate()
	c.mu.Unlock()
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
func (c *client) ID() string {
	return c.uid
}

// UserID returns ID of user. No locking here as User() can not be called before
// we set user value in connect command - after this we only read this value.
func (c *client) UserID() string {
	return c.user
}

func (c *client) Encoding() proto.Encoding {
	return c.encoding
}

func (c *client) Transport() conns.Transport {
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
	if c.Encoding() == proto.EncodingJSON {
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

	c.transport.Send(proto.NewPreparedReply(&proto.Reply{
		Result: result,
	}, c.encoding))

	return nil
}

// clean called when connection was closed to make different clean up
// actions for a client
func (c *client) Close(advice *proto.Disconnect) error {
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
				c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error unsubscribing client from channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			}
		}
	}

	if c.authenticated {
		err := c.node.RemoveClient(c)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error removing client", map[string]interface{}{"user": c.user, "client": c.uid, "error": err.Error()}))
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

	if advice != nil && advice.Reason != "" {
		c.node.Logger().Log(logging.NewEntry(logging.DEBUG, "closing client connection", map[string]interface{}{"client": c.uid, "user": c.user, "reason": advice.Reason}))
	}

	c.transport.Close(advice)

	if c.node.Mediator() != nil && c.node.Mediator().DisconnectHandler != nil {
		c.node.Mediator().DisconnectHandler(c.ctx, &events.DisconnectContext{
			EventContext: events.EventContext{
				Client: c,
			},
		})
	}

	return nil
}

func (c *client) info(ch string) *proto.ClientInfo {
	connInfo := c.connInfo
	channelInfo, _ := c.channelInfo[ch]
	return &proto.ClientInfo{
		User:     c.user,
		Client:   c.uid,
		ConnInfo: connInfo,
		ChanInfo: channelInfo,
	}
}

// Handle dispatches Command into correct command handler.
func (c *client) Handle(command *proto.Command) (*proto.Reply, *proto.Disconnect) {

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, nil
	}

	var replyRes []byte
	var replyErr *proto.Error
	var disconnect *proto.Disconnect

	method := command.Method
	params := command.Params

	if method != "connect" && !c.authenticated {
		// Client must send connect command first.
		replyErr = proto.ErrUnauthorized
	} else {
		started := time.Now()
		switch method {
		case "connect":
			replyRes, replyErr, disconnect = c.handleConnect(params)
		case "refresh":
			replyRes, replyErr, disconnect = c.handleRefresh(params)
		case "subscribe":
			replyRes, replyErr, disconnect = c.handleSubscribe(params)
		case "unsubscribe":
			replyRes, replyErr, disconnect = c.handleUnsubscribe(params)
		case "publish":
			replyRes, replyErr, disconnect = c.handlePublish(params)
		case "presence":
			replyRes, replyErr, disconnect = c.handlePresence(params)
		case "presence_stats":
			replyRes, replyErr, disconnect = c.handlePresenceStats(params)
		case "history":
			replyRes, replyErr, disconnect = c.handleHistory(params)
		case "ping":
			replyRes, replyErr, disconnect = c.handlePing(params)
		case "rpc":
			mediator := c.node.Mediator()
			if mediator != nil && mediator.RPCHandler != nil {
				rpcReply, err := mediator.RPCHandler(c.ctx, &events.RPCContext{
					Data: params,
				})
				if err == nil {
					disconnect = rpcReply.Disconnect
					replyRes = rpcReply.Result
					replyErr = rpcReply.Error
				}
			} else {
				replyRes, replyErr = nil, proto.ErrNotAvailable
			}
		default:
			replyRes, replyErr = nil, proto.ErrMethodNotFound
		}
		commandDurationSummary.WithLabelValues(method).Observe(float64(time.Since(started).Seconds()))
	}

	if disconnect != nil {
		return nil, disconnect
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

	c.mu.RLock()
	ttl := c.exp - time.Now().Unix()
	c.mu.RUnlock()
	if ttl > 0 {
		// connection was successfully refreshed.
		return
	}

	c.Close(&proto.Disconnect{Reason: "expired", Reconnect: true})
	return
}

func (c *client) handleConnect(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodeConnect(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding connect", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.connectCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodeConnectResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding connect", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handleRefresh(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodeRefresh(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding refresh", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.refreshCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodeRefreshResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding refresh", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handleSubscribe(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodeSubscribe(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding subscribe", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.subscribeCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodeSubscribeResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding subscribe", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handleUnsubscribe(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodeUnsubscribe(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding unsubscribe", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.unsubscribeCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodeUnsubscribeResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding unsubscribe", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handlePublish(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodePublish(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding publish", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.publishCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodePublishResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding publish", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handlePresence(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodePresence(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding presence", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.presenceCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodePresenceResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding presence", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handlePresenceStats(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodePresenceStats(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding presence stats", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.presenceStatsCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodePresenceStatsResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding presence stats", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handleHistory(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodeHistory(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding history", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.historyCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodeHistoryResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding history", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

func (c *client) handlePing(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := proto.GetParamsDecoder(c.encoding).DecodePing(params)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error decoding ping", map[string]interface{}{"error": err.Error()}))
		return nil, nil, proto.DisconnectBadRequest
	}
	resp, disconnect := c.pingCmd(cmd)
	if resp.Error != nil || disconnect != nil {
		return nil, resp.Error, disconnect
	}
	var replyRes []byte
	if resp.Result != nil {
		replyRes, err = proto.GetResultEncoder(c.encoding).EncodePingResult(resp.Result)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error encoding ping", map[string]interface{}{"error": err.Error()}))
			return nil, nil, proto.DisconnectServerError
		}
	}
	return replyRes, nil, nil
}

// connectCmd handles connect command from client - client must send this
// command immediately after establishing Websocket or SockJS connection with
// Centrifugo
func (c *client) connectCmd(cmd *proto.ConnectRequest) (*proto.ConnectResponse, *proto.Disconnect) {

	resp := &proto.ConnectResponse{}

	if c.authenticated {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "client already authenticated", map[string]interface{}{"client": c.uid, "user": c.user}))
		return nil, proto.DisconnectBadRequest
	}

	config := c.node.Config()

	secret := config.Secret
	insecure := config.ClientInsecure
	closeDelay := config.ClientExpiredCloseDelay
	clientExpire := config.ClientExpire
	version := c.node.Version()
	userConnectionLimit := config.UserConnectionLimit

	var credentials *Credentials
	if val := c.ctx.Value(CredentialsContextKey); val != nil {
		if creds, ok := val.(*Credentials); ok {
			credentials = creds
		}
	}

	var expired bool
	var ttl uint32

	if credentials != nil {
		c.user = credentials.UserID
		c.connInfo = credentials.Info
		c.exp = credentials.Exp
		ttl = uint32(c.exp - time.Now().Unix())
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
				c.node.Logger().Log(logging.NewEntry(logging.ERROR, "invalid sign", map[string]interface{}{"user": user, "client": c.uid}))
				return resp, proto.DisconnectInvalidSign
			}
			timestamp, err := strconv.Atoi(exp)
			if err != nil {
				c.node.Logger().Log(logging.NewEntry(logging.ERROR, "invalid timestamp", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
				return resp, proto.DisconnectBadRequest
			}
			c.exp = int64(timestamp)
		}

		c.user = user
		c.opts = opts
		if len(info) > 0 {
			c.connInfo = proto.Raw(info)
		}
	}

	if userConnectionLimit > 0 && c.user != "" && len(c.node.Hub().UserConnections(c.user)) >= userConnectionLimit {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "limit of connections for user reached", map[string]interface{}{"user": c.user, "client": c.uid, "limit": userConnectionLimit}))
		resp.Error = proto.ErrLimitExceeded
		return resp, nil
	}

	if clientExpire && !insecure {
		ttl = uint32(c.exp - time.Now().Unix())
		if ttl <= 0 {
			expired = true
			ttl = 0
		}
	}

	res := &proto.ConnectResult{
		Version: version,
		Expires: clientExpire,
		TTL:     ttl,
		Expired: expired,
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

	err := c.node.AddClient(c)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error adding client", map[string]interface{}{"client": c.uid, "error": err.Error()}))
		return resp, proto.DisconnectServerError
	}

	if clientExpire && !expired {
		duration := closeDelay + time.Duration(ttl)*time.Second
		c.expireTimer = time.AfterFunc(duration, c.expire)
	}

	resp.Result.Client = c.uid

	// TODO: check locking.
	if c.node.Mediator() != nil && c.node.Mediator().ConnectHandler != nil {
		c.node.Mediator().ConnectHandler(c.ctx, &events.ConnectContext{
			EventContext: events.EventContext{
				Client: c,
			},
		})
	}

	return resp, nil
}

// refreshCmd handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime option set.
func (c *client) refreshCmd(cmd *proto.RefreshRequest) (*proto.RefreshResponse, *proto.Disconnect) {

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
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "invalid refresh sign", map[string]interface{}{"user": user, "client": c.uid}))
		return resp, proto.DisconnectInvalidSign
	}

	timestamp, err := strconv.Atoi(exp)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "invalid refresh timestamp", map[string]interface{}{"user": user, "client": c.uid, "error": err.Error()}))
		return resp, proto.DisconnectBadRequest
	}

	closeDelay := config.ClientExpiredCloseDelay
	clientExpire := config.ClientExpire
	version := c.node.Version()

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
			c.connInfo = proto.Raw(info)
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
func (c *client) subscribeCmd(cmd *proto.SubscribeRequest) (*proto.SubscribeResponse, *proto.Disconnect) {

	channel := cmd.Channel
	if channel == "" {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "channel required for subscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, proto.DisconnectBadRequest
	}

	resp := &proto.SubscribeResponse{}

	config := c.node.Config()
	secret := config.Secret
	channelMaxLength := config.ChannelMaxLength
	channelLimit := config.ClientChannelLimit
	insecure := config.ClientInsecure

	res := &proto.SubscribeResult{}

	if len(channel) > channelMaxLength {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "channel too long", map[string]interface{}{"max": channelMaxLength, "channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = proto.ErrLimitExceeded
		return resp, nil
	}

	if len(c.channels) >= channelLimit {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "maximum limit of channels per client reached", map[string]interface{}{"limit": channelLimit, "user": c.user, "client": c.uid}))
		resp.Error = proto.ErrLimitExceeded
		return resp, nil
	}

	if _, ok := c.channels[channel]; ok {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "client already subscribed on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = proto.ErrAlreadySubscribed
		return resp, nil
	}

	if !c.node.UserAllowed(channel, c.user) || !c.node.ClientAllowed(channel, c.uid) {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "user not allowed to subscribe on channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid}))
		resp.Error = proto.ErrPermissionDenied
		return resp, nil
	}

	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		resp.Error = proto.ErrNamespaceNotFound
		return resp, nil
	}

	if !chOpts.Anonymous && c.user == "" && !insecure {
		resp.Error = proto.ErrPermissionDenied
		return resp, nil
	}

	if c.node.PrivateChannel(channel) {
		// private channel - subscription must be properly signed
		if string(c.uid) != string(cmd.Client) {
			resp.Error = proto.ErrPermissionDenied
			return resp, nil
		}
		isValid := auth.CheckChannelSign(secret, string(cmd.Client), string(channel), cmd.Info, cmd.Sign)
		if !isValid {
			resp.Error = proto.ErrPermissionDenied
			return resp, nil
		}
		if len(cmd.Info) > 0 {
			c.channelInfo[channel] = proto.Raw(cmd.Info)
		}
	}

	if c.node.Mediator() != nil && c.node.Mediator().SubscribeHandler != nil {
		c.node.Mediator().SubscribeHandler(c.ctx, &events.SubscribeContext{
			EventContext: events.EventContext{
				Client: c,
			},
			Channel: channel,
		})
	}

	c.channels[channel] = struct{}{}

	err := c.node.AddSubscription(channel, c)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error adding subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
		return nil, proto.DisconnectServerError
	}

	info := c.info(channel)

	if chOpts.Presence {
		err = c.node.AddPresence(channel, c.uid, info)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error adding presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			return nil, proto.DisconnectServerError
		}
	}

	if chOpts.HistoryRecover {
		if cmd.Recover {
			// Client provided subscribe request with recover flag on. Try to recover missed messages
			// automatically from history (we suppose here that history configured wisely) based on
			// provided last message id value.
			messages, err := c.node.History(channel)
			if err != nil {
				c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error recovering messages", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
				res.Publications = nil
			} else {
				recoveredMessages, recovered := recoverMessages(cmd.Last, messages)
				res.Publications = recoveredMessages
				res.Recovered = recovered
			}
		} else {
			// Client don't want to recover messages yet, we just return last message id to him here.
			lastMessageID, err := c.node.LastMessageID(channel)
			if err != nil {
				c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error getting last message ID for channel", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			} else {
				res.Last = lastMessageID
			}
		}
	}

	if chOpts.JoinLeave {
		join := &proto.Join{
			Info: *info,
		}
		go c.node.PublishJoin(channel, join, &chOpts)
	}

	resp.Result = res
	return resp, nil
}

func (c *client) unsubscribe(channel string) error {
	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		return proto.ErrNamespaceNotFound
	}

	info := c.info(channel)

	_, ok = c.channels[channel]
	if ok {
		delete(c.channels, channel)

		if chOpts.Presence {
			err := c.node.RemovePresence(channel, c.uid)
			if err != nil {
				c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error removing channel presence", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			}
		}

		if chOpts.JoinLeave {
			leave := &proto.Leave{
				Info: *info,
			}
			c.node.PublishLeave(channel, leave, &chOpts)
		}

		err := c.node.RemoveSubscription(channel, c)
		if err != nil {
			c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error removing subscription", map[string]interface{}{"channel": channel, "user": c.user, "client": c.uid, "error": err.Error()}))
			return err
		}
	}

	if c.node.Mediator() != nil && c.node.Mediator().UnsubscribeHandler != nil {
		c.node.Mediator().UnsubscribeHandler(c.ctx, &events.UnsubscribeContext{
			EventContext: events.EventContext{
				Client: c,
			},
			Channel: channel,
		})
	}

	return nil
}

// unsubscribeCmd handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *client) unsubscribeCmd(cmd *proto.UnsubscribeRequest) (*proto.UnsubscribeResponse, *proto.Disconnect) {

	channel := cmd.Channel
	if channel == "" {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "channel required for unsubscribe", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, proto.DisconnectBadRequest
	}

	resp := &proto.UnsubscribeResponse{}

	err := c.unsubscribe(channel)
	if err != nil {
		resp.Error = proto.ErrInternalServerError
		return resp, nil
	}

	return resp, nil
}

// publishCmd handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) publishCmd(cmd *proto.PublishRequest) (*proto.PublishResponse, *proto.Disconnect) {

	ch := cmd.Channel
	data := cmd.Data

	if string(ch) == "" || len(data) == 0 {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "channel and data required for publish", map[string]interface{}{"user": c.user, "client": c.uid}))
		return nil, proto.DisconnectBadRequest
	}

	resp := &proto.PublishResponse{}

	if _, ok := c.channels[ch]; !ok {
		resp.Error = proto.ErrPermissionDenied
		return resp, nil
	}

	info := c.info(ch)

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "attempt to subscribe on non-existing namespace", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid}))
		resp.Error = proto.ErrNamespaceNotFound
		return resp, nil
	}

	insecure := c.node.Config().ClientInsecure

	if !chOpts.Publish && !insecure {
		resp.Error = proto.ErrPermissionDenied
		return resp, nil
	}

	publication := &proto.Publication{
		Data: data,
		Info: info,
	}

	if c.node.Mediator() != nil && c.node.Mediator().PublishHandler != nil {
		c.node.Mediator().PublishHandler(c.ctx, &events.PublishContext{
			EventContext: events.EventContext{
				Client: c,
			},
			Channel:     ch,
			Publication: publication,
		})
	}

	err := <-c.node.Publish(ch, publication, &chOpts)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error publishing", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = proto.ErrInternalServerError
		return resp, nil
	}

	return resp, nil
}

// presenceCmd handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *client) presenceCmd(cmd *proto.PresenceRequest) (*proto.PresenceResponse, *proto.Disconnect) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, proto.DisconnectBadRequest
	}

	resp := &proto.PresenceResponse{}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = proto.ErrNamespaceNotFound
		return resp, nil
	}

	if _, ok := c.channels[ch]; !ok {
		resp.Error = proto.ErrPermissionDenied
		return resp, nil
	}

	if !chOpts.Presence {
		resp.Error = proto.ErrNotAvailable
		return resp, nil
	}

	presence, err := c.node.Presence(ch)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error getting presence", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = proto.ErrInternalServerError
		return resp, nil
	}

	resp.Result = &proto.PresenceResult{
		Presence: presence,
	}

	return resp, nil
}

// presenceStatsCmd ...
func (c *client) presenceStatsCmd(cmd *proto.PresenceStatsRequest) (*proto.PresenceStatsResponse, *proto.Disconnect) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, proto.DisconnectBadRequest
	}

	resp := &proto.PresenceStatsResponse{}

	if _, ok := c.channels[ch]; !ok {
		resp.Error = proto.ErrPermissionDenied
		return resp, nil
	}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = proto.ErrNamespaceNotFound
		return resp, nil
	}

	if !chOpts.Presence || !chOpts.PresenceStats {
		resp.Error = proto.ErrNotAvailable
		return resp, nil
	}

	presence, err := c.node.Presence(ch)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error getting presence", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = proto.ErrInternalServerError
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
		NumClients: uint64(numClients),
		NumUsers:   uint64(numUsers),
	}

	return resp, nil
}

// historyCmd handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag).
func (c *client) historyCmd(cmd *proto.HistoryRequest) (*proto.HistoryResponse, *proto.Disconnect) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, proto.DisconnectBadRequest
	}

	resp := &proto.HistoryResponse{}

	if _, ok := c.channels[ch]; !ok {
		resp.Error = proto.ErrPermissionDenied
		return resp, nil
	}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		resp.Error = proto.ErrNamespaceNotFound
		return resp, nil
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		resp.Error = proto.ErrNotAvailable
		return resp, nil
	}

	publications, err := c.node.History(ch)
	if err != nil {
		c.node.Logger().Log(logging.NewEntry(logging.ERROR, "error getting history", map[string]interface{}{"channel": ch, "user": c.user, "client": c.uid, "error": err.Error()}))
		resp.Error = proto.ErrInternalServerError
		return resp, nil
	}

	resp.Result = &proto.HistoryResult{
		Publications: publications,
	}

	return resp, nil
}

// pingCmd handles ping command from client.
func (c *client) pingCmd(cmd *proto.PingRequest) (*proto.PingResponse, *proto.Disconnect) {
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
