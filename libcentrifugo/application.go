// Package libcentrifugo is a real-time core for Centrifugo server.
package libcentrifugo

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/gorilla/securecookie"
	"github.com/centrifugal/centrifugo/Godeps/_workspace/src/github.com/nu7hatch/gouuid"
)

// Application is a heart of Centrifugo – it internally manages client and admin hubs,
// maintains information about other Centrifugo nodes, keeps references to
// config, engine, metrics etc.
type Application struct {
	sync.RWMutex

	// unique id for this application (node).
	uid string

	// started is unix time of node start.
	started int64

	// nodes is a map with information about nodes known.
	nodes map[string]NodeInfo

	// nodesMu allows to synchronize access to nodes.
	nodesMu sync.Mutex

	// hub to manage client connections.
	clients *clientHub

	// hub to manage admin connections.
	admins *adminHub

	// engine to use - in memory or redis.
	engine Engine

	// config for application.
	config *Config

	// mediator allows integrate libcentrifugo Application with external go code.
	mediator Mediator

	// shutdown is a flag which is only true when application is going to shut down.
	shutdown bool

	// metrics holds various counters and timers different parts of Centrifugo update.
	metrics *metricsRegistry
}

// Stats contains state and metrics information from running Centrifugo nodes.
type Stats struct {
	Nodes           []NodeInfo `json:"nodes"`
	MetricsInterval int64      `json:"metrics_interval"`
}

// NodeInfo contains information and statistics about Centrifugo node.
type NodeInfo struct {
	UID        string `json:"uid"`
	Name       string `json:"name"`
	Goroutines int    `json:"num_goroutine"`
	Clients    int    `json:"num_clients"`
	Unique     int    `json:"num_unique_clients"`
	Channels   int    `json:"num_channels"`
	Started    int64  `json:"started_at"`
	Gomaxprocs int    `json:"gomaxprocs"`
	NumCPU     int    `json:"num_cpu"`
	Metrics
	updated int64
}

// NewApplication returns new Application instance, the only required argument is
// config, structure and engine must be set via corresponding methods.
func NewApplication(config *Config) (*Application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	app := &Application{
		uid:     uid.String(),
		config:  config,
		clients: newClientHub(),
		admins:  newAdminHub(),
		nodes:   make(map[string]NodeInfo),
		started: time.Now().Unix(),
		metrics: newMetricsRegistry(),
	}
	return app, nil
}

// Run performs all startup actions. At moment must be called once on start after engine and
// structure set.
func (app *Application) Run() error {
	if err := app.engine.run(); err != nil {
		return err
	}
	go app.sendNodePingMsg()
	go app.cleanNodeInfo()
	go app.updateMetrics()
	return nil
}

// Shutdown sets shutdown flag and does various connection clean ups (at moment only unsubscribes
// all clients from all channels and disconnects them).
func (app *Application) Shutdown() {
	app.Lock()
	app.shutdown = true
	app.Unlock()
	app.clients.shutdown()
}

func (app *Application) updateMetricsOnce() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	cpu, err := cpuUsage()
	if err != nil {
		logger.DEBUG.Println(err)
	}

	app.metrics.Lock()
	app.metrics.metrics.CPU = cpu
	app.metrics.metrics.MemSys = int64(mem.Sys)
	app.metrics.metrics.NumMsgPublished = app.metrics.numMsgPublished.Count()
	app.metrics.metrics.NumMsgQueued = app.metrics.numMsgQueued.Count()
	app.metrics.metrics.NumMsgSent = app.metrics.numMsgSent.Count()
	app.metrics.metrics.NumAPIRequests = app.metrics.numAPIRequests.Count()
	app.metrics.metrics.NumClientRequests = app.metrics.numClientRequests.Count()
	app.metrics.metrics.TimeAPIMean = int64(app.metrics.timeAPI.Mean())
	app.metrics.metrics.TimeClientMean = int64(app.metrics.timeClient.Mean())
	app.metrics.metrics.TimeAPIMax = int64(app.metrics.timeAPI.Max())
	app.metrics.metrics.TimeClientMax = int64(app.metrics.timeClient.Max())
	app.metrics.metrics.BytesClientIn = app.metrics.bytesClientIn.Count()
	app.metrics.metrics.BytesClientOut = app.metrics.bytesClientOut.Count()
	app.metrics.Unlock()

	app.metrics.numMsgPublished.Clear()
	app.metrics.numMsgQueued.Clear()
	app.metrics.numMsgSent.Clear()
	app.metrics.numAPIRequests.Clear()
	app.metrics.numClientRequests.Clear()
	app.metrics.bytesClientIn.Clear()
	app.metrics.bytesClientOut.Clear()
}

func (app *Application) updateMetrics() {
	for {
		app.RLock()
		interval := app.config.NodeMetricsInterval
		app.RUnlock()
		time.Sleep(interval)
		app.updateMetricsOnce()
	}
}

func (app *Application) sendNodePingMsg() {
	for {
		err := app.pubPing()
		if err != nil {
			logger.CRITICAL.Println(err)
		}
		app.RLock()
		interval := app.config.NodePingInterval
		app.RUnlock()
		time.Sleep(interval)
	}
}

func (app *Application) cleanNodeInfo() {
	for {
		app.RLock()
		delay := app.config.NodeInfoMaxDelay
		app.RUnlock()

		app.nodesMu.Lock()
		for uid, info := range app.nodes {
			if time.Now().Unix()-info.updated > int64(delay.Seconds()) {
				delete(app.nodes, uid)
			}
		}
		app.nodesMu.Unlock()

		app.RLock()
		interval := app.config.NodeInfoCleanInterval
		app.RUnlock()

		time.Sleep(interval)
	}
}

// SetConfig binds config to application.
func (app *Application) SetConfig(c *Config) {
	app.Lock()
	defer app.Unlock()
	app.config = c
	if app.config.Insecure {
		logger.WARN.Println("libcentrifugo: application in INSECURE MODE")
	}
}

// SetEngine binds engine to application.
func (app *Application) SetEngine(e Engine) {
	app.Lock()
	defer app.Unlock()
	app.engine = e
}

// SetMediator binds mediator to application.
func (app *Application) SetMediator(m Mediator) {
	app.Lock()
	defer app.Unlock()
	app.mediator = m
}

func (app *Application) channels() ([]Channel, error) {
	app.RLock()
	controlChID := app.config.ControlChannel
	adminChID := app.config.AdminChannel
	app.RUnlock()
	chIDs, err := app.engine.channels()
	if err != nil {
		return []Channel{}, err
	}
	prefix := app.channelIDPrefix()
	channels := make([]Channel, 0, len(chIDs))
	for _, chID := range chIDs {
		if strings.HasPrefix(string(chID), prefix) && chID != controlChID && chID != adminChID {
			channels = append(channels, Channel(string(chID)[len(prefix):]))
		}
	}
	return channels, nil
}

func (app *Application) stats() Stats {
	app.nodesMu.Lock()
	nodes := make([]NodeInfo, len(app.nodes))
	i := 0
	for _, info := range app.nodes {
		nodes[i] = info
		i++
	}
	app.nodesMu.Unlock()

	app.RLock()
	interval := app.config.NodeMetricsInterval
	app.RUnlock()

	return Stats{
		MetricsInterval: int64(interval.Seconds()),
		Nodes:           nodes,
	}
}

// handleMsg called when new message of any type received by this node.
// It looks at channel and decides which message handler to call.
func (app *Application) handleMsg(chID ChannelID, message []byte) error {
	switch chID {
	case app.config.ControlChannel:
		return app.controlMsg(message)
	case app.config.AdminChannel:
		return app.adminMsg(message)
	default:
		return app.clientMsg(chID, message)
	}
}

// controlMsg handles messages from control channel - control
// messages used for internal communication between nodes to share state
// or commands.
func (app *Application) controlMsg(message []byte) error {

	var cmd controlCommand
	err := json.Unmarshal(message, &cmd)
	if err != nil {
		logger.ERROR.Println(err)
		return err
	}

	if cmd.UID == app.uid {
		// Sent by this node.
		return nil
	}

	method := cmd.Method
	params := cmd.Params

	switch method {
	case "ping":
		var cmd pingControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidMessage
		}
		return app.pingCmd(&cmd)
	case "unsubscribe":
		var cmd unsubscribeControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidMessage
		}
		return app.unsubscribeUser(cmd.User, cmd.Channel)
	case "disconnect":
		var cmd disconnectControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidMessage
		}
		return app.disconnectUser(cmd.User)
	default:
		logger.ERROR.Println("unknown control message method", method)
		return ErrInvalidMessage
	}
	panic("unreachable")
}

// adminMsg handles messages from admin channel - those messages
// must be delivered to all admins connected to this node.
func (app *Application) adminMsg(message []byte) error {
	return app.admins.broadcast(message)
}

// clientMsg handles messages published by web application or client
// into channel. The goal of this method to deliver this message to all clients
// on this node subscribed on channel.
func (app *Application) clientMsg(chID ChannelID, message []byte) error {
	return app.clients.broadcast(chID, message)
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it.
func (app *Application) pubControl(method string, params []byte) error {

	raw := json.RawMessage(params)

	message := controlCommand{
		UID:    app.uid,
		Method: method,
		Params: &raw,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.ControlChannel, messageBytes)
}

// pubAdmin publishes message into admin channel so all running
// nodes will receive it and send to admins connected.
func (app *Application) pubAdmin(message []byte) error {
	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.AdminChannel, message)
}

// Publish sends a message to all clients subscribed on channel with provided data, client and ClientInfo.
func (app *Application) Publish(ch Channel, data []byte, client ConnID, info *ClientInfo) error {

	if string(ch) == "" || len(data) == 0 {
		return ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return err
	}

	err = app.pubClient(ch, chOpts, data, client, info)
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInternalServerError
	}

	return nil
}

// publish sends a message into channel with provided data, client and client info.
// If fromClient argument is true then internally this method will check client permission to
// publish into this channel.
func (app *Application) publish(ch Channel, data []byte, client ConnID, info *ClientInfo, fromClient bool) error {

	if string(ch) == "" || len(data) == 0 {
		return ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return err
	}

	app.RLock()
	insecure := app.config.Insecure
	app.RUnlock()

	if fromClient && !chOpts.Publish && !insecure {
		return ErrPermissionDenied
	}

	if app.mediator != nil {
		// If mediator is set then we don't need to publish message
		// immediately as mediator will decide itself what to do with it.
		pass := app.mediator.Message(ch, data, client, info)
		if !pass {
			return nil
		}
	}

	err = app.pubClient(ch, chOpts, data, client, info)
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInternalServerError
	}

	return nil
}

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel.
func (app *Application) pubClient(ch Channel, chOpts ChannelOptions, data []byte, client ConnID, info *ClientInfo) error {

	message, err := newMessage(ch, data, client, info)
	if err != nil {
		return err
	}

	if chOpts.Watch {
		resp := newResponse("message")
		resp.Body = &adminMessageBody{
			Message: message,
		}
		messageBytes, err := json.Marshal(resp)
		if err != nil {
			logger.ERROR.Println(err)
		} else {
			err = app.pubAdmin(messageBytes)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	chID := app.channelID(ch)

	resp := newResponse("message")
	resp.Body = message

	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	err = app.engine.publish(chID, byteMessage)
	if err != nil {
		return err
	}

	if chOpts.HistorySize > 0 && chOpts.HistoryLifetime > 0 {
		histOpts := addHistoryOpts{
			Size:     chOpts.HistorySize,
			Lifetime: chOpts.HistoryLifetime,
		}
		err = app.addHistory(ch, message, histOpts)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	app.metrics.numMsgPublished.Inc(1)

	return nil
}

// pubJoinLeave allows to publish join message into channel when
// someone subscribes on it or leave message when someone unsubscribed from channel.
func (app *Application) pubJoinLeave(ch Channel, method string, info ClientInfo) error {
	chID := app.channelID(ch)
	resp := newResponse(method)
	resp.Body = &JoinLeaveBody{
		Channel: ch,
		Data:    info,
	}
	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return app.engine.publish(chID, byteMessage)
}

// pubPing sends control ping message to all nodes - this message
// contains information about current node.
func (app *Application) pubPing() error {
	app.RLock()
	defer app.RUnlock()
	app.metrics.RLock()
	info := NodeInfo{
		UID:        app.uid,
		Name:       app.config.Name,
		Clients:    app.nClients(),
		Unique:     app.nUniqueClients(),
		Channels:   app.nChannels(),
		Started:    app.started,
		Goroutines: runtime.NumGoroutine(),
		NumCPU:     runtime.NumCPU(),
		Gomaxprocs: runtime.GOMAXPROCS(-1),
		Metrics:    *app.metrics.metrics,
	}
	cmd := &pingControlCommand{Info: info}
	app.metrics.RUnlock()

	err := app.pingCmd(cmd)
	if err != nil {
		logger.ERROR.Println(err)
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return app.pubControl("ping", cmdBytes)
}

// pubUnsubscribe publishes unsubscribe control message to all nodes – so all
// nodes could unsubscribe user from channel.
func (app *Application) pubUnsubscribe(user UserID, ch Channel) error {

	cmd := &unsubscribeControlCommand{
		User:    user,
		Channel: ch,
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return app.pubControl("unsubscribe", cmdBytes)
}

// pubDisconnect publishes disconnect control message to all nodes – so all
// nodes could disconnect user from Centrifugo.
func (app *Application) pubDisconnect(user UserID) error {

	cmd := &disconnectControlCommand{
		User: user,
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return app.pubControl("disconnect", cmdBytes)
}

// pingCmd handles ping control command i.e. updates information about known nodes.
func (app *Application) pingCmd(cmd *pingControlCommand) error {
	info := cmd.Info
	info.updated = time.Now().Unix()
	app.nodesMu.Lock()
	app.nodes[info.UID] = info
	app.nodesMu.Unlock()
	return nil
}

func (app *Application) channelIDPrefix() string {
	app.RLock()
	defer app.RUnlock()
	return app.config.ChannelPrefix + ".channel."
}

// channelID returns internal name of channel.
func (app *Application) channelID(ch Channel) ChannelID {
	return ChannelID(app.channelIDPrefix() + string(ch))
}

// addConn registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand.
func (app *Application) addConn(c clientConn) error {
	return app.clients.add(c)
}

// removeConn removes client connection from connection registry.
func (app *Application) removeConn(c clientConn) error {
	return app.clients.remove(c)
}

// addSub registers subscription of connection on channel in both
// engine and clientSubscriptionHub.
func (app *Application) addSub(ch Channel, c clientConn) error {
	chID := app.channelID(ch)
	first, err := app.clients.addSub(chID, c)
	if err != nil {
		return err
	}
	if first {
		return app.engine.subscribe(chID)
	}
	return nil
}

// removeSub removes subscription of connection on channel
// from both engine and clientSubscriptionHub.
func (app *Application) removeSub(ch Channel, c clientConn) error {
	chID := app.channelID(ch)
	empty, err := app.clients.removeSub(chID, c)
	if err != nil {
		return err
	}
	if empty {
		return app.engine.unsubscribe(chID)
	}
	return nil
}

// Unsubscribe unsubscribes user from channel, if channel is equal to empty
// string then user will be unsubscribed from all channels.
func (app *Application) Unsubscribe(user UserID, ch Channel) error {

	if string(user) == "" {
		return ErrInvalidMessage
	}

	if string(ch) != "" {
		_, err := app.channelOpts(ch)
		if err != nil {
			return err
		}
	}

	// First unsubscribe on this node.
	err := app.unsubscribeUser(user, ch)
	if err != nil {
		return ErrInternalServerError
	}
	// Second send unsubscribe control message to other nodes.
	err = app.pubUnsubscribe(user, ch)
	if err != nil {
		return ErrInternalServerError
	}
	return nil
}

// unsubscribeUser unsubscribes user from channel on this node. If channel
// is an empty string then user will be unsubscribed from all channels.
func (app *Application) unsubscribeUser(user UserID, ch Channel) error {
	userConnections := app.clients.userConnections(user)
	for _, c := range userConnections {
		var channels []Channel
		if string(ch) == "" {
			// unsubscribe from all channels
			channels = c.channels()
		} else {
			channels = []Channel{ch}
		}

		for _, channel := range channels {
			err := c.unsubscribe(channel)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Disconnect allows to close all user connections to Centrifugo. Note that user still
// can try to reconnect to the server after being disconnected.
func (app *Application) Disconnect(user UserID) error {

	if string(user) == "" {
		return ErrInvalidMessage
	}

	// first disconnect user from this node
	err := app.disconnectUser(user)
	if err != nil {
		return ErrInternalServerError
	}
	// second send disconnect control message to other nodes
	err = app.pubDisconnect(user)
	if err != nil {
		return ErrInternalServerError
	}
	return nil
}

// disconnectUser closes client connections of user on current node.
func (app *Application) disconnectUser(user UserID) error {
	userConnections := app.clients.userConnections(user)
	for _, c := range userConnections {
		err := c.close("disconnect")
		if err != nil {
			return err
		}
	}
	return nil
}

// namespaceKey returns namespace key from channel name if exists.
func (app *Application) namespaceKey(ch Channel) NamespaceKey {
	cTrim := strings.TrimPrefix(string(ch), app.config.PrivateChannelPrefix)
	if strings.Contains(cTrim, app.config.NamespaceChannelBoundary) {
		parts := strings.SplitN(cTrim, app.config.NamespaceChannelBoundary, 2)
		return NamespaceKey(parts[0])
	}
	return NamespaceKey("")
}

// channelOpts returns channel options for channel using current application structure.
func (app *Application) channelOpts(ch Channel) (ChannelOptions, error) {
	app.RLock()
	defer app.RUnlock()
	nk := app.namespaceKey(ch)
	return app.config.channelOpts(nk)
}

// addPresence proxies presence adding to engine.
func (app *Application) addPresence(ch Channel, uid ConnID, info ClientInfo) error {
	chID := app.channelID(ch)
	return app.engine.addPresence(chID, uid, info)
}

// removePresence proxies presence removing to engine.
func (app *Application) removePresence(ch Channel, uid ConnID) error {
	chID := app.channelID(ch)
	return app.engine.removePresence(chID, uid)
}

// Presence returns a map of active clients in project channel.
func (app *Application) Presence(ch Channel) (map[ConnID]ClientInfo, error) {

	if string(ch) == "" {
		return map[ConnID]ClientInfo{}, ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return map[ConnID]ClientInfo{}, err
	}

	if !chOpts.Presence {
		return map[ConnID]ClientInfo{}, ErrNotAvailable
	}

	chID := app.channelID(ch)

	presence, err := app.engine.presence(chID)
	if err != nil {
		return map[ConnID]ClientInfo{}, ErrInternalServerError
	}
	return presence, nil
}

// addHistory proxies history message adding to engine.
func (app *Application) addHistory(ch Channel, message Message, opts addHistoryOpts) error {
	chID := app.channelID(ch)
	return app.engine.addHistory(chID, message, opts)
}

// History returns a slice of last messages published into project channel.
func (app *Application) History(ch Channel) ([]Message, error) {

	if string(ch) == "" {
		return []Message{}, ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return []Message{}, err
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		return []Message{}, ErrNotAvailable
	}

	chID := app.channelID(ch)

	history, err := app.engine.history(chID, historyOpts{})
	if err != nil {
		return []Message{}, ErrInternalServerError
	}
	return history, nil
}

func (app *Application) lastMessageID(ch Channel) (MessageID, error) {
	chID := app.channelID(ch)
	history, err := app.engine.history(chID, historyOpts{Limit: 1})
	if err != nil {
		return MessageID(""), err
	}
	if len(history) == 0 {
		return MessageID(""), nil
	}
	return history[0].UID, nil
}

// privateChannel checks if channel private and therefore subscription
// request on it must be properly signed on web application backend.
func (app *Application) privateChannel(ch Channel) bool {
	app.RLock()
	defer app.RUnlock()
	return strings.HasPrefix(string(ch), app.config.PrivateChannelPrefix)
}

// userAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (app *Application) userAllowed(ch Channel, user UserID) bool {
	app.RLock()
	defer app.RUnlock()
	if !strings.Contains(string(ch), app.config.UserChannelBoundary) {
		return true
	}
	parts := strings.Split(string(ch), app.config.UserChannelBoundary)
	allowedUsers := strings.Split(parts[len(parts)-1], app.config.UserChannelSeparator)
	for _, allowedUser := range allowedUsers {
		if string(user) == allowedUser {
			return true
		}
	}
	return false
}

// clientAllowed checks if client can subscribe on channel - as channel
// can contain special part in the end to indicate which client allowed
// to subscribe on it.
func (app *Application) clientAllowed(ch Channel, client ConnID) bool {
	app.RLock()
	defer app.RUnlock()
	if !strings.Contains(string(ch), app.config.ClientChannelBoundary) {
		return true
	}
	parts := strings.Split(string(ch), app.config.ClientChannelBoundary)
	allowedClient := parts[len(parts)-1]
	if string(client) == allowedClient {
		return true
	}
	return false
}

// addAdminConn registers an admin connection in adminConnectionHub.
func (app *Application) addAdminConn(c adminConn) error {
	return app.admins.add(c)
}

// removeAdminConn admin connection from adminConnectionHub.
func (app *Application) removeAdminConn(c adminConn) error {
	return app.admins.remove(c)
}

// nChannels returns total amount of active channels on this node.
func (app *Application) nChannels() int {
	return app.clients.nChannels()
}

// nClients returns total amount of client connections to this node.
func (app *Application) nClients() int {
	return app.clients.nClients()
}

// nUniqueClients returns total amount of unique client connections to this node.
func (app *Application) nUniqueClients() int {
	return app.clients.nUniqueClients()
}

const (
	// AuthTokenKey is a key for admin authorization token.
	AuthTokenKey = "token"
	// AuthTokenValue is a value for secure admin authorization token.
	AuthTokenValue = "authorized"
)

func (app *Application) adminAuthToken() (string, error) {
	app.RLock()
	secret := app.config.WebSecret
	app.RUnlock()
	if secret == "" {
		logger.ERROR.Println("provide web_secret in configuration")
		return "", ErrInternalServerError
	}
	s := securecookie.New([]byte(secret), nil)
	return s.Encode(AuthTokenKey, AuthTokenValue)
}

// checkAdminAuthToken checks admin connection token which Centrifugo returns after admin login.
func (app *Application) checkAdminAuthToken(token string) error {

	app.RLock()
	insecure := app.config.InsecureWeb
	secret := app.config.WebSecret
	app.RUnlock()

	if insecure {
		return nil
	}

	if secret == "" {
		logger.ERROR.Println("provide web_secret in configuration")
		return ErrUnauthorized
	}

	if token == "" {
		return ErrUnauthorized
	}

	s := securecookie.New([]byte(secret), nil)
	var val string
	err := s.Decode(AuthTokenKey, token, &val)
	if err != nil {
		return ErrUnauthorized
	}

	if val != AuthTokenValue {
		return ErrUnauthorized
	}
	return nil
}
