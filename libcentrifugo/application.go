// Package libcentrifugo is a real-time core for Centrifugo server.
package libcentrifugo

import (
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/gorilla/securecookie"
	"github.com/satori/go.uuid"
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
	nodes map[string]nodeInfo

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

	// shutdownCh is a channel which is closed when shutdown happens.
	shutdownCh chan struct{}

	// metrics holds various counters and timers different parts of Centrifugo update.
	metrics *metricsRegistry
}

// NewApplication returns new Application instance, the only required argument is
// config, structure and engine must be set via corresponding methods.
func NewApplication(config *Config) (*Application, error) {
	app := &Application{
		uid:        uuid.NewV4().String(),
		config:     config,
		clients:    newClientHub(),
		admins:     newAdminHub(),
		nodes:      make(map[string]nodeInfo),
		started:    time.Now().Unix(),
		metrics:    newMetricsRegistry(),
		shutdownCh: make(chan struct{}),
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
	if app.shutdown {
		app.Unlock()
		return
	}
	app.shutdown = true
	close(app.shutdownCh)
	app.Unlock()
	app.clients.shutdown()
}

func (app *Application) updateMetricsOnce() {
	app.metrics.UpdateSnapshot()
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
	return app.engine.channels()
}

func (app *Application) stats() serverStats {
	app.nodesMu.Lock()
	nodes := make([]nodeInfo, len(app.nodes))
	i := 0
	for _, info := range app.nodes {
		nodes[i] = info
		i++
	}
	app.nodesMu.Unlock()

	app.RLock()
	interval := app.config.NodeMetricsInterval
	app.RUnlock()

	return serverStats{
		MetricsInterval: int64(interval.Seconds()),
		Nodes:           nodes,
	}
}

func (app *Application) node() nodeInfo {
	app.nodesMu.Lock()
	info, ok := app.nodes[app.uid]
	if !ok {
		logger.WARN.Println("node called but no local node info yet, returning garbage")
	}
	app.nodesMu.Unlock()

	info.metrics = *app.metrics.GetRawMetrics()

	return info
}

// controlMsg handles messages from control channel - control messages used for internal
// communication between nodes to share state or commands.
func (app *Application) controlMsg(cmd *ControlMessage) error {

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
}

// adminMsg handlesadmin message broadcasting it to all admins connected to this node.
func (app *Application) adminMsg(message *AdminMessage) error {
	app.admins.RLock()
	hasAdmins := len(app.admins.connections) > 0
	app.admins.RUnlock()
	if !hasAdmins {
		return nil
	}
	resp := newAPIAdminMessageResponse(message.Params)
	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return app.admins.broadcast(byteMessage)
}

// clientMsg handles messages published by web application or client into channel.
// The goal of this method to deliver this message to all clients on this node subscribed
// on channel.
func (app *Application) clientMsg(ch Channel, message *Message) error {
	numSubscribers := app.clients.numSubscribers(ch)
	hasCurrentSubscribers := numSubscribers > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := newClientMessage()
	resp.Body = *message
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.broadcast(ch, byteMessage)
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

	errCh := app.pubClient(ch, chOpts, data, client, info)
	err = <-errCh
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInternalServerError
	}

	return nil
}

func makeErrChan(err error) <-chan error {
	ret := make(chan error, 1)
	ret <- err
	return ret
}

// publish sends a message into channel with provided data, client and client info.
// If fromClient argument is true then internally this method will check client permission to
// publish into this channel.
func (app *Application) publishAsync(ch Channel, data []byte, client ConnID, info *ClientInfo, fromClient bool) <-chan error {
	if string(ch) == "" || len(data) == 0 {
		return makeErrChan(ErrInvalidMessage)
	}

	chOpts, err := app.channelOpts(ch)
	if err != nil {
		return makeErrChan(err)
	}

	app.RLock()
	insecure := app.config.Insecure
	app.RUnlock()

	if fromClient && !chOpts.Publish && !insecure {
		return makeErrChan(ErrPermissionDenied)
	}

	if app.mediator != nil {
		// If mediator is set then we don't need to publish message
		// immediately as mediator will decide itself what to do with it.
		pass := app.mediator.Message(ch, data, client, info)
		if !pass {
			return makeErrChan(nil)
		}
	}

	return app.pubClient(ch, chOpts, data, client, info)
}

// publish sends a message into channel with provided data, client and client info.
// If fromClient argument is true then internally this method will check client permission to
// publish into this channel.
func (app *Application) publish(ch Channel, data []byte, client ConnID, info *ClientInfo, fromClient bool) error {
	return <-app.publishAsync(ch, data, client, info, fromClient)
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it.
func (app *Application) pubControl(method string, params []byte) error {
	return <-app.engine.publishControl(newControlMessage(app.uid, method, params))
}

// pubAdmin publishes message to admins.
func (app *Application) pubAdmin(method string, params []byte) <-chan error {
	return app.engine.publishAdmin(newAdminMessage(method, params))
}

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel.
func (app *Application) pubClient(ch Channel, chOpts ChannelOptions, data []byte, client ConnID, info *ClientInfo) <-chan error {
	message := newMessage(ch, data, client, info)
	app.metrics.NumMsgPublished.Inc()
	if chOpts.Watch {
		byteMessage, err := json.Marshal(message)
		if err != nil {
			logger.ERROR.Println(err)
		} else {
			app.pubAdmin("message", byteMessage)
		}
	}
	return app.engine.publishMessage(ch, message, &chOpts)
}

// pubJoin allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (app *Application) pubJoin(ch Channel, info ClientInfo) error {
	return <-app.engine.publishJoin(ch, newJoinMessage(ch, info))
}

// pubLeave allows to publish join message into channel when someone subscribes on it
// or leave message when someone unsubscribes from channel.
func (app *Application) pubLeave(ch Channel, info ClientInfo) error {
	return <-app.engine.publishLeave(ch, newLeaveMessage(ch, info))
}

func (app *Application) joinMsg(ch Channel, message *JoinMessage) error {
	hasCurrentSubscribers := app.clients.numSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := newClientJoinMessage()
	resp.Body = *message
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.broadcast(ch, byteMessage)
}

func (app *Application) leaveMsg(ch Channel, message *LeaveMessage) error {
	hasCurrentSubscribers := app.clients.numSubscribers(ch) > 0
	if !hasCurrentSubscribers {
		return nil
	}
	resp := newClientLeaveMessage()
	resp.Body = *message
	byteMessage, err := resp.Marshal()
	if err != nil {
		return err
	}
	return app.clients.broadcast(ch, byteMessage)
}

// pubPing sends control ping message to all nodes - this message
// contains information about current node.
func (app *Application) pubPing() error {
	app.RLock()
	info := nodeInfo{
		UID:        app.uid,
		Name:       app.config.Name,
		Clients:    app.nClients(),
		Unique:     app.nUniqueClients(),
		Channels:   app.nChannels(),
		Started:    app.started,
		Goroutines: runtime.NumGoroutine(),
		NumCPU:     runtime.NumCPU(),
		Gomaxprocs: runtime.GOMAXPROCS(-1),
		metrics:    *app.metrics.GetSnapshotMetrics(),
	}
	app.RUnlock()
	cmd := &pingControlCommand{Info: info}

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
	first, err := app.clients.addSub(ch, c)
	if err != nil {
		return err
	}
	if first {
		return app.engine.subscribe(ch)
	}
	return nil
}

// removeSub removes subscription of connection on channel
// from both engine and clientSubscriptionHub.
func (app *Application) removeSub(ch Channel, c clientConn) error {
	empty, err := app.clients.removeSub(ch, c)
	if err != nil {
		return err
	}
	if empty {
		return app.engine.unsubscribe(ch)
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
	return app.engine.addPresence(ch, uid, info)
}

// removePresence proxies presence removing to engine.
func (app *Application) removePresence(ch Channel, uid ConnID) error {
	return app.engine.removePresence(ch, uid)
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

	presence, err := app.engine.presence(ch)
	if err != nil {
		logger.ERROR.Println(err)
		return map[ConnID]ClientInfo{}, ErrInternalServerError
	}
	return presence, nil
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

	history, err := app.engine.history(ch, 0)
	if err != nil {
		logger.ERROR.Println(err)
		return []Message{}, ErrInternalServerError
	}
	return history, nil
}

func (app *Application) lastMessageID(ch Channel) (MessageID, error) {
	history, err := app.engine.history(ch, 1)
	if err != nil {
		return MessageID(""), err
	}
	if len(history) == 0 {
		return MessageID(""), nil
	}
	return MessageID(history[0].UID), nil
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
	secret := app.config.AdminSecret
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
	insecure := app.config.InsecureAdmin
	secret := app.config.AdminSecret
	app.RUnlock()

	if insecure {
		return nil
	}

	if secret == "" {
		logger.ERROR.Println("provide admin_secret in configuration")
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
