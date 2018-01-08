package conns

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/lib/auth"
	"github.com/centrifugal/centrifugo/lib/internal/queue"
	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/metrics"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
	clientproto "github.com/centrifugal/centrifugo/lib/proto/client"

	"github.com/satori/go.uuid"
)

func init() {
	metricsRegistry := metrics.DefaultRegistry

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

// client represents client connection to Centrifugo - at moment
// this can be Websocket or SockJS connection. Transport of incoming
// connection abstracted away via Session interface.
type client struct {
	mu             sync.RWMutex
	ctx            context.Context
	node           *node.Node
	session        Session
	uid            string
	user           string
	timestamp      int64
	authenticated  bool
	defaultInfo    proto.Raw
	channelInfo    map[string]proto.Raw
	channels       map[string]struct{}
	messages       queue.Queue
	closeCh        chan struct{}
	closed         bool
	staleTimer     *time.Timer
	expireTimer    *time.Timer
	presenceTimer  *time.Timer
	maxQueueSize   int
	maxRequestSize int
	sendFinished   chan struct{}
	encoding       clientproto.Encoding
	paramsDecoder  clientproto.ParamsDecoder
	resultEncoder  clientproto.ResultEncoder
}

// CommandHandler must handle incoming command from client.
type CommandHandler func(ctx context.Context, method string, params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect)

// New creates new client connection.
func New(ctx context.Context, n *node.Node, s Session, enc clientproto.Encoding, credentials *Credentials) node.Client {
	config := n.Config()
	staleCloseDelay := config.StaleConnectionCloseDelay
	queueInitialCapacity := config.ClientQueueInitialCapacity
	maxQueueSize := config.ClientQueueMaxSize
	maxRequestSize := config.ClientRequestMaxSize

	c := client{
		uid:            uuid.NewV4().String(),
		node:           n,
		session:        s,
		messages:       queue.New(queueInitialCapacity),
		closeCh:        make(chan struct{}),
		sendFinished:   make(chan struct{}),
		maxQueueSize:   maxQueueSize,
		maxRequestSize: maxRequestSize,
		paramsDecoder:  clientproto.GetParamsDecoder(enc),
		resultEncoder:  clientproto.GetResultEncoder(enc),
	}

	go c.sendMessages()

	if staleCloseDelay > 0 {
		c.staleTimer = time.AfterFunc(staleCloseDelay, c.closeUnauthenticated)
	}

	return &c
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
		// Write timeout must be implemented inside session Send method.
		// Slow client connections will be closed eventually anyway after
		// exceeding client max queue size.
		err := c.session.Send(msg)
		if err != nil {
			// Close in goroutine to let this function return.
			go c.Close(&proto.Disconnect{Reason: "error sending message", Reconnect: true})
			return
		}
		metrics.DefaultRegistry.Counters.Inc("client_num_msg_sent")
		metrics.DefaultRegistry.Counters.Add("client_bytes_out", int64(len(msg)))
	}
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
func (c *client) UID() string {
	return c.uid
}

// No lock here as User() can not be called before we set user value in connect command.
// After this we only read this value.
func (c *client) User() string {
	return c.user
}

func (c *client) Encoding() clientproto.Encoding {
	return c.encoding
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

	// TODO: send AsyncUnsubscribeMessage to this client.
	// encoder := clientproto.GetReplyEncoder(c.encoding)
	// defer clientproto.PutReplyEncoder(c.encoding, encoder)
	// unsubscribe := &proto.Unsubscribe{}
	// message := proto.NewUnsubscribeMessage(ch)

	return nil
}

func (c *client) enqueue(data []byte) error {
	ok := c.messages.Add(data)
	if !ok {
		return nil
	}
	metrics.DefaultRegistry.Counters.Inc("client_num_msg_queued")
	if c.messages.Size() > c.maxQueueSize {
		// Close in goroutine to not block message broadcast.
		go c.Close(&proto.Disconnect{Reason: "slow", Reconnect: true})
		return nil
	}
	return nil
}

func (c *client) Send(data []byte) error {
	return c.enqueue(data)
}

// clean called when connection was closed to make different clean up
// actions for a client
func (c *client) Close(advice *proto.Disconnect) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	close(c.closeCh)
	c.closed = true

	c.messages.Close()

	if len(c.channels) > 0 {
		// unsubscribe from all channels
		for channel := range c.channels {
			err := c.unsubscribe(channel)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	if c.authenticated {
		err := c.node.RemoveClient(c)
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

	if advice != nil && advice.Reason != "" {
		logger.DEBUG.Printf("Closing connection %s (user %s): %s", c.uid, c.user, advice.Reason)
	}

	c.session.Close(advice)

	return nil
}

func (c *client) info(ch string) *proto.ClientInfo {
	defaultInfo := c.defaultInfo
	channelInfo, _ := c.channelInfo[ch]
	return &proto.ClientInfo{
		User:     c.user,
		Client:   c.uid,
		ConnInfo: defaultInfo,
		ChanInfo: channelInfo,
	}
}

func (c *client) Handle(data []byte) error {
	started := time.Now()
	defer func() {
		metrics.DefaultRegistry.HDRHistograms.RecordMicroseconds("client_api", time.Now().Sub(started))
	}()
	metrics.DefaultRegistry.Counters.Inc("client_api_num_requests")
	metrics.DefaultRegistry.Counters.Add("client_bytes_in", int64(len(data)))

	if len(data) == 0 {
		logger.ERROR.Println("empty client request received")
		c.Close(&proto.Disconnect{Reason: proto.ErrInvalidData.Error(), Reconnect: false})
		return proto.ErrInvalidData
	} else if len(data) > c.maxRequestSize {
		logger.ERROR.Println("client request exceeds max request size limit")
		c.Close(&proto.Disconnect{Reason: proto.ErrLimitExceeded.Error(), Reconnect: false})
		return proto.ErrLimitExceeded
	}

	encoder := clientproto.GetReplyEncoder(c.encoding)
	defer clientproto.PutReplyEncoder(c.encoding, encoder)

	decoder := clientproto.GetCommandDecoder(c.encoding)
	defer clientproto.PutCommandDecoder(c.encoding, decoder)

	for {
		cmd, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.ERROR.Printf("error decoding request: %v", err)
			c.Close(&proto.Disconnect{Reason: "malformed request", Reconnect: false})
			return proto.ErrInvalidData
		}
		rep, disconnect := c.handleCmd(cmd)
		if disconnect != nil {
			logger.ERROR.Printf("disconnect after handling command %v: %v", cmd, disconnect)
			c.Close(disconnect)
			return fmt.Errorf("disconnect: %s", disconnect.Reason)
		}
		err = encoder.Encode(rep)
		if err != nil {
			logger.ERROR.Printf("error encoding reply %v: %v", rep, err)
			c.Close(&proto.Disconnect{Reason: "internal error", Reconnect: true})
			return err
		}
	}
	return c.enqueue(encoder.Finish())
}

// handleCmd dispatches clientCommand into correct command handler
func (c *client) handleCmd(command *clientproto.Command) (*clientproto.Reply, *proto.Disconnect) {

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
		default:
			replyRes, replyErr = nil, proto.ErrMethodNotFound
		}
	}

	if disconnect != nil {
		return nil, disconnect
	}

	rep := &clientproto.Reply{
		ID:     command.ID,
		Result: replyRes,
		Error:  replyErr,
	}

	return rep, nil
}

func (c *client) expire() {
	config := c.node.Config()
	connLifetime := config.ConnLifetime

	if connLifetime <= 0 {
		return
	}

	c.mu.RLock()
	timeToExpire := c.timestamp + connLifetime - time.Now().Unix()
	c.mu.RUnlock()
	if timeToExpire > 0 {
		// connection was successfully refreshed.
		return
	}

	c.Close(&proto.Disconnect{Reason: "expired", Reconnect: true})
	return
}

func (c *client) handleConnect(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodeConnect(params)
	if err != nil {
		logger.ERROR.Printf("error decoding connect: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.connectCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodeConnectResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding connect: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handleRefresh(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodeRefresh(params)
	if err != nil {
		logger.ERROR.Printf("error decoding refresh: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.refreshCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodeRefreshResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding refresh: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handleSubscribe(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodeSubscribe(params)
	if err != nil {
		logger.ERROR.Printf("error decoding subscribe: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.subscribeCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodeSubscribeResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding subscribe: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handleUnsubscribe(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodeUnsubscribe(params)
	if err != nil {
		logger.ERROR.Printf("error decoding unsubscribe: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.unsubscribeCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodeUnsubscribeResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding unsubscribe: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handlePublish(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodePublish(params)
	if err != nil {
		logger.ERROR.Printf("error decoding publish: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.publishCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodePublishResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding publish: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handlePresence(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodePresence(params)
	if err != nil {
		logger.ERROR.Printf("error decoding presence: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.presenceCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodePresenceResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding presence: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handlePresenceStats(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodePresenceStats(params)
	if err != nil {
		logger.ERROR.Printf("error decoding presence stats: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.presenceStatsCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodePresenceStatsResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding presence stats: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handleHistory(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodeHistory(params)
	if err != nil {
		logger.ERROR.Printf("error decoding history: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.historyCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodeHistoryResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding history: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

func (c *client) handlePing(params proto.Raw) (proto.Raw, *proto.Error, *proto.Disconnect) {
	cmd, err := c.paramsDecoder.DecodePing(params)
	if err != nil {
		logger.ERROR.Printf("error decoding ping: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}
	res, replyErr, disconnect := c.pingCmd(cmd)
	if replyErr != nil || disconnect != nil {
		return nil, replyErr, disconnect
	}
	replyRes, err := c.resultEncoder.EncodePingResult(res)
	if err != nil {
		logger.ERROR.Printf("error encoding ping: %v", err)
		return nil, nil, proto.DisconnectServerError
	}
	return replyRes, nil, nil
}

// connectCmd handles connect command from client - client must send this
// command immediately after establishing Websocket or SockJS connection with
// Centrifugo
func (c *client) connectCmd(cmd *clientproto.Connect) (*clientproto.ConnectResult, *proto.Error, *proto.Disconnect) {

	metrics.DefaultRegistry.Counters.Inc("client_num_connect")

	if c.authenticated {
		logger.ERROR.Println("connect error: client already authenticated")
		return nil, proto.ErrInvalidData, nil
	}

	user := cmd.User
	info := cmd.Info
	opts := cmd.Opts

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
		timestamp = cmd.Time
		token = cmd.Token
	} else {
		timestamp = ""
		token = ""
	}

	if !insecure {
		isValid := auth.CheckClientToken(secret, string(user), timestamp, info, opts, token)
		if !isValid {
			logger.ERROR.Println("invalid token for user", user)
			return nil, nil, proto.DisconnectInvalidToken
		}
		ts, err := strconv.Atoi(timestamp)
		if err != nil {
			logger.ERROR.Printf("invalid timestamp: %v", err)
			return nil, nil, proto.DisconnectInvalidMessage
		}
		c.timestamp = int64(ts)
	} else {
		c.timestamp = time.Now().Unix()
	}

	if userConnectionLimit > 0 && user != "" && len(c.node.Hub().UserConnections(user)) >= userConnectionLimit {
		logger.ERROR.Printf("limit of connections %d for user %s reached", userConnectionLimit, user)
		return nil, proto.ErrLimitExceeded, nil
	}

	c.user = user

	res := &clientproto.ConnectResult{
		Version: version,
		Expires: connLifetime > 0,
		TTL:     uint32(connLifetime),
	}

	var timeToExpire int64

	if connLifetime > 0 && !insecure {
		timeToExpire = c.timestamp + connLifetime - time.Now().Unix()
		if timeToExpire <= 0 {
			res.Expired = true
			return res, nil, nil
		}
	}

	c.authenticated = true
	if len(info) > 0 {
		c.defaultInfo = proto.Raw(info)
	}
	c.channels = map[string]struct{}{}
	c.channelInfo = map[string]proto.Raw{}

	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}

	c.addPresenceUpdate()

	err := c.node.AddClient(c)
	if err != nil {
		logger.ERROR.Printf("error adding client: %v", err)
		return nil, nil, proto.DisconnectServerError
	}

	if timeToExpire > 0 {
		duration := closeDelay + time.Duration(timeToExpire)*time.Second
		c.expireTimer = time.AfterFunc(duration, c.expire)
	}

	res.Client = c.uid
	return res, nil, nil
}

// refreshCmd handle refresh command to update connection with new
// timestamp - this is only required when connection lifetime option set.
func (c *client) refreshCmd(cmd *clientproto.Refresh) (*clientproto.RefreshResult, *proto.Error, *proto.Disconnect) {

	user := cmd.User
	info := cmd.Info
	timestamp := cmd.Time
	opts := cmd.Opts
	token := cmd.Token

	config := c.node.Config()
	secret := config.Secret

	isValid := auth.CheckClientToken(secret, string(user), timestamp, info, opts, token)
	if !isValid {
		logger.ERROR.Println("invalid refresh token for user", user)
		return nil, nil, proto.DisconnectInvalidToken
	}

	ts, err := strconv.Atoi(timestamp)
	if err != nil {
		logger.ERROR.Printf("invalid timestamp: %v", err)
		return nil, nil, proto.DisconnectInvalidMessage
	}

	closeDelay := config.ExpiredConnectionCloseDelay
	connLifetime := config.ConnLifetime
	version := c.node.Version()

	res := &clientproto.RefreshResult{
		Version: version,
		Expires: connLifetime > 0,
		TTL:     uint32(connLifetime),
		Client:  c.uid,
	}

	if connLifetime > 0 {
		// connection check enabled
		timeToExpire := int64(ts) + connLifetime - time.Now().Unix()
		if timeToExpire > 0 {
			// connection refreshed, update client timestamp and set new expiration timeout
			c.timestamp = int64(ts)
			c.defaultInfo = proto.Raw(info)
			if c.expireTimer != nil {
				c.expireTimer.Stop()
			}
			duration := time.Duration(timeToExpire)*time.Second + closeDelay
			c.expireTimer = time.AfterFunc(duration, c.expire)
		} else {
			res.Expired = true
		}
	}
	return res, nil, nil
}

func recoverMessages(last string, messages []*proto.Message) ([]*proto.Message, bool) {
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
func (c *client) subscribeCmd(cmd *clientproto.Subscribe) (*clientproto.SubscribeResult, *proto.Error, *proto.Disconnect) {

	metrics.DefaultRegistry.Counters.Inc("client_num_subscribe")

	channel := cmd.Channel
	if channel == "" {
		logger.ERROR.Printf("channel not found in subscribe cmd")
		return nil, nil, proto.DisconnectInvalidMessage
	}

	config := c.node.Config()
	secret := config.Secret
	maxChannelLength := config.MaxChannelLength
	channelLimit := config.ClientChannelLimit
	insecure := config.Insecure

	res := &clientproto.SubscribeResult{}

	if len(channel) > maxChannelLength {
		logger.ERROR.Printf("channel too long: max %d, got %d", maxChannelLength, len(channel))
		return nil, proto.ErrLimitExceeded, nil
	}

	if len(c.channels) >= channelLimit {
		logger.ERROR.Printf("maximum limit of channels per client reached: %d", channelLimit)
		return nil, proto.ErrLimitExceeded, nil
	}

	if _, ok := c.channels[channel]; ok {
		logger.ERROR.Printf("client already subscribed on channel %s", channel)
		return nil, proto.ErrAlreadySubscribed, nil
	}

	if !c.node.UserAllowed(channel, c.user) || !c.node.ClientAllowed(channel, c.uid) {
		logger.ERROR.Printf("user %s not allowed to subscribe on channel %s", c.User(), channel)
		return nil, proto.ErrPermissionDenied, nil
	}

	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		return nil, proto.ErrNamespaceNotFound, nil
	}

	if !chOpts.Anonymous && c.user == "" && !insecure {
		return nil, proto.ErrPermissionDenied, nil
	}

	if c.node.PrivateChannel(channel) {
		// private channel - subscription must be properly signed
		if string(c.uid) != string(cmd.Client) {
			return nil, proto.ErrPermissionDenied, nil
		}
		isValid := auth.CheckChannelSign(secret, string(cmd.Client), string(channel), cmd.Info, cmd.Sign)
		if !isValid {
			return nil, proto.ErrPermissionDenied, nil
		}
		if len(cmd.Info) > 0 {
			c.channelInfo[channel] = proto.Raw(cmd.Info)
		}
	}

	c.channels[channel] = struct{}{}

	err := c.node.AddSubscription(channel, c)
	if err != nil {
		logger.ERROR.Printf("error adding new subscription: %v", err)
		return nil, nil, proto.DisconnectServerError
	}

	info := c.info(channel)

	if chOpts.Presence {
		err = c.node.AddPresence(channel, c.uid, info)
		if err != nil {
			logger.ERROR.Printf("error adding presence: %v", err)
			return nil, nil, proto.DisconnectServerError
		}
	}

	if chOpts.HistoryRecover {
		if cmd.Recover {
			// Client provided subscribe request with recover flag on. Try to recover missed messages
			// automatically from history (we suppose here that history configured wisely) based on
			// provided last message id value.
			messages, err := c.node.History(channel)
			if err != nil {
				logger.ERROR.Printf("can't recover messages for channel %s: %s", string(channel), err)
				res.Messages = []*proto.Message{}
			} else {
				recoveredMessages, recovered := recoverMessages(cmd.Last, messages)
				res.Messages = recoveredMessages
				res.Recovered = recovered
			}
		} else {
			// Client don't want to recover messages yet, we just return last message id to him here.
			lastMessageID, err := c.node.LastMessageID(channel)
			if err != nil {
				logger.ERROR.Println(err)
			} else {
				res.Last = lastMessageID
			}
		}
	}

	if chOpts.JoinLeave {
		join := &proto.Join{
			Info: *info,
		}
		c.node.PublishJoin(channel, join, &chOpts)
	}

	return res, nil, nil
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
				logger.ERROR.Printf("error removing presence: %v", err)
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
			logger.ERROR.Printf("error removing subscription: %v", err)
			return err
		}
	}
	return nil
}

// unsubscribeCmd handles unsubscribe command from client - it allows to
// unsubscribe connection from channel
func (c *client) unsubscribeCmd(cmd *clientproto.Unsubscribe) (*clientproto.UnsubscribeResult, *proto.Error, *proto.Disconnect) {

	channel := cmd.Channel
	if channel == "" {
		logger.ERROR.Printf("channel not found in unsubscribe cmd")
		return nil, nil, proto.DisconnectInvalidMessage
	}

	res := &clientproto.UnsubscribeResult{}

	err := c.unsubscribe(channel)
	if err != nil {
		return nil, proto.ErrInternalServerError, nil
	}

	return res, nil, nil
}

// publishCmd handles publish command - clients can publish messages into
// channels themselves if `publish` allowed by channel options. In most cases clients not
// allowed to publish into channels directly - web application publishes messages
// itself via HTTP API or Redis.
func (c *client) publishCmd(cmd *clientproto.Publish) (*clientproto.PublishResult, *proto.Error, *proto.Disconnect) {

	channel := cmd.Channel
	data := cmd.Data

	res := &clientproto.PublishResult{}

	if string(channel) == "" || len(data) == 0 {
		logger.ERROR.Printf("channel and data required for publish")
		return nil, nil, proto.DisconnectInvalidMessage
	}

	if _, ok := c.channels[channel]; !ok {
		return nil, proto.ErrPermissionDenied, nil
	}

	info := c.info(channel)

	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		logger.ERROR.Printf("can't get channel options for channel: %s", channel)
		return nil, proto.ErrNamespaceNotFound, nil
	}

	insecure := c.node.Config().Insecure

	if !chOpts.Publish && !insecure {
		return nil, proto.ErrPermissionDenied, nil
	}

	metrics.DefaultRegistry.Counters.Inc("client_num_msg_published")

	publication := &proto.Publication{
		Data: data,
		Info: info,
	}

	err := <-c.node.Publish(channel, publication, &chOpts)
	if err != nil {
		logger.ERROR.Printf("error publishing message: %v", err)
		return nil, proto.ErrInternalServerError, nil
	}

	return res, nil, nil
}

// presenceCmd handles presence command - it shows which clients
// are subscribed on channel at this moment. This method also checks if
// presence information turned on for channel (based on channel options
// for namespace or project)
func (c *client) presenceCmd(cmd *clientproto.Presence) (*clientproto.PresenceResult, *proto.Error, *proto.Disconnect) {

	// TODO: all error checks must be done here and not inside node method.

	channel := cmd.Channel

	if _, ok := c.channels[channel]; !ok {
		return nil, proto.ErrPermissionDenied, nil
	}

	presence, err := c.node.Presence(channel)
	if err != nil {
		logger.ERROR.Printf("error getting presence: %v", err)
		return nil, proto.ErrInternalServerError, nil
	}

	res := &clientproto.PresenceResult{
		Data: presence,
	}

	return res, nil, nil
}

// presenceStatsCmd ...
func (c *client) presenceStatsCmd(cmd *clientproto.PresenceStats) (*clientproto.PresenceStatsResult, *proto.Error, *proto.Disconnect) {

	channel := cmd.Channel

	if _, ok := c.channels[channel]; !ok {
		return nil, proto.ErrPermissionDenied, nil
	}

	chOpts, ok := c.node.ChannelOpts(channel)
	if !ok {
		return nil, proto.ErrNamespaceNotFound, nil
	}

	if !chOpts.Presence || !chOpts.PresenceStats {
		return nil, proto.ErrNotAvailable, nil
	}

	presence, err := c.node.Presence(channel)
	if err != nil {
		logger.ERROR.Printf("error getting presence: %v", err)
		return nil, proto.ErrInternalServerError, nil
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

	res := &clientproto.PresenceStatsResult{
		NumClients: uint64(numClients),
		NumUsers:   uint64(numUsers),
	}

	return res, nil, nil
}

// historyCmd handles history command - it shows last M messages published
// into channel. M is history size and can be configured for project or namespace
// via channel options. Also this method checks that history available for channel
// (also determined by channel options flag).
func (c *client) historyCmd(cmd *clientproto.History) (*clientproto.HistoryResult, *proto.Error, *proto.Disconnect) {

	ch := cmd.Channel

	if string(ch) == "" {
		return nil, nil, proto.DisconnectInvalidMessage
	}

	if _, ok := c.channels[ch]; !ok {
		return nil, proto.ErrPermissionDenied, nil
	}

	chOpts, ok := c.node.ChannelOpts(ch)
	if !ok {
		return nil, proto.ErrNamespaceNotFound, nil
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		return nil, proto.ErrNotAvailable, nil
	}

	history, err := c.node.History(ch)
	if err != nil {
		logger.ERROR.Printf("error getting history: %v", err)
		return nil, proto.ErrInternalServerError, nil
	}

	res := &clientproto.HistoryResult{
		Data: history,
	}

	return res, nil, nil
}

// pingCmd handles ping command from client.
func (c *client) pingCmd(cmd *clientproto.Ping) (*clientproto.PingResult, *proto.Error, *proto.Disconnect) {
	var res *clientproto.PingResult
	if cmd.Data != "" {
		res.Data = cmd.Data
	}
	return res, nil, nil
}
