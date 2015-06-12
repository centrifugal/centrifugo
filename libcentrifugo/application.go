// Package libcentrifugo is a real-time core for Centrifugo server.
package libcentrifugo

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/gorilla/securecookie"
	"github.com/nu7hatch/gouuid"
)

// Application is a heart of Centrifugo – it internally manages connection, subscription
// and admin hubs, maintains information about other Centrifugo nodes, keeps references to
// config, engine and structure.
type Application struct {
	sync.RWMutex

	// unique id for this application (node).
	uid string

	// started is unix time of node start.
	started int64

	// nodes is a map with information about nodes known.
	nodes map[string]*nodeInfo
	// nodesMu allows to synchronize access to nodes.
	nodesMu sync.Mutex

	// hub to manage client connections.
	connHub *clientHub
	// hub to manage client subscriptions.
	subs *subHub
	// hub to manage admin connections.
	admins *adminHub

	// engine to use - in memory or redis.
	engine Engine

	// reference to structure to work with projects and namespaces.
	structure *Structure

	// config for application.
	config *Config

	// shuttdown is a flag which is only true when application is going to shut down.
	shutdown bool
}

type nodeInfo struct {
	Uid      string `json:"uid"`
	Name     string `json:"name"`
	Clients  int    `json:"clients"`
	Unique   int    `json:"unique"`
	Channels int    `json:"channels"`
	Started  int64  `json:"started"`
	Updated  int64  `json:"-"`
}

// NewApplication returns new Application instance, the only required argument is
// config, structure and engine must be set via corresponding methods.
func NewApplication(c *Config) (*Application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	app := &Application{
		uid:     uid.String(),
		nodes:   make(map[string]*nodeInfo),
		connHub: newClientHub(),
		subs:    newSubHub(),
		admins:  newAdminHub(),
		started: time.Now().Unix(),
		config:  c,
	}
	return app, nil
}

// Run performs all startup actions. At moment must be called once on start after engine and
// structure set.
func (app *Application) Run() {
	app.RLock()
	if app.config.Insecure {
		logger.WARN.Println("libcentrifugo: application in INSECURE MODE")
	}
	app.RUnlock()
	go app.sendPingMsg()
	go app.cleanNodeInfo()
}

// Shutdown sets shutdown flag and does various connection clean ups (at moment only unsubscribes
// all clients from all channels and disconnects them).
func (app *Application) Shutdown() {
	app.Lock()
	app.shutdown = true
	app.Unlock()
	app.connHub.shutdown()
}

func (app *Application) sendPingMsg() {
	for {
		err := app.pubPing()
		if err != nil {
			logger.CRITICAL.Println(err)
		}
		app.RLock()
		interval := app.config.NodePingInterval
		app.RUnlock()
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func (app *Application) cleanNodeInfo() {
	for {
		app.RLock()
		delay := app.config.NodeInfoMaxDelay
		app.RUnlock()

		app.nodesMu.Lock()
		for uid, info := range app.nodes {
			if time.Now().Unix()-info.Updated > delay {
				delete(app.nodes, uid)
			}
		}
		app.nodesMu.Unlock()

		app.RLock()
		interval := app.config.NodeInfoCleanInterval
		app.RUnlock()

		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// SetConfig binds config to application
func (app *Application) SetConfig(c *Config) {
	app.Lock()
	defer app.Unlock()
	app.config = c
	if app.config.Insecure {
		logger.WARN.Println("libcentrifugo: application in INSECURE MODE")
	}
}

// SetEngine binds structure to application
func (app *Application) SetStructure(s *Structure) {
	app.Lock()
	defer app.Unlock()
	app.structure = s
}

// SetEngine binds engine to application
func (app *Application) SetEngine(e Engine) {
	app.Lock()
	defer app.Unlock()
	app.engine = e
}

// handleMsg called when new message of any type received by this node.
// It looks at channel and decides which message handler to call
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
// or commands
func (app *Application) controlMsg(message []byte) error {

	var cmd controlCommand
	err := json.Unmarshal(message, &cmd)
	if err != nil {
		logger.ERROR.Println(err)
		return err
	}

	if cmd.Uid == app.uid {
		// sent by this node
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
		return app.unsubscribeUser(cmd.Project, cmd.User, cmd.Channel)
	case "disconnect":
		var cmd disconnectControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidMessage
		}
		return app.disconnectUser(cmd.Project, cmd.User)
	default:
		logger.ERROR.Println("unknown control message method", method)
		return ErrInvalidMessage
	}
	panic("unreachable")
}

// adminMsg handles messages from admin channel - those messages
// must be delivered to all admins connected to this node
func (app *Application) adminMsg(message []byte) error {
	return app.admins.broadcast(string(message))
}

// clientMsg handles messages published by web application or client
// into channel. The goal of this method to deliver this message to all clients
// on this node subscribed on channel
func (app *Application) clientMsg(chID ChannelID, message []byte) error {
	return app.subs.broadcast(chID, string(message))
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it
func (app *Application) pubControl(method string, params []byte) error {

	raw := json.RawMessage(params)

	message := controlCommand{
		Uid:    app.uid,
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
// nodes will receive it and send to admins connected
func (app *Application) pubAdmin(message []byte) error {
	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.AdminChannel, message)
}

// Message represents client message
type Message struct {
	Uid       string           `json:"uid"`
	Timestamp string           `json:"timestamp"`
	Info      *ClientInfo      `json:"info"`
	Channel   Channel          `json:"channel"`
	Data      *json.RawMessage `json:"data"`
	Client    ConnID           `json:"client"`
}

// Publish sends a message into project channel with provided data, client and client info.
// If asClient argument is true then internally this method will check client permission to
// publish into this channel.
func (app *Application) Publish(pk ProjectKey, ch Channel, data []byte, client ConnID, info *ClientInfo, asClient bool) error {

	if string(ch) == "" || len(data) == 0 {
		return ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(pk, ch)
	if err != nil {
		return err
	}

	app.RLock()
	insecure := app.config.Insecure
	app.RUnlock()

	if asClient && !chOpts.Publish && !insecure {
		return ErrPermissionDenied
	}

	err = app.pubClient(pk, ch, chOpts, data, client, info)
	if err != nil {
		logger.ERROR.Println(err)
		return ErrInternalServerError
	}

	return nil
}

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel
func (app *Application) pubClient(pk ProjectKey, ch Channel, chOpts ChannelOptions, data []byte, client ConnID, info *ClientInfo) error {

	uid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	raw := json.RawMessage(data)

	message := Message{
		Uid:       uid.String(),
		Timestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Info:      info,
		Channel:   ch,
		Data:      &raw,
		Client:    client,
	}

	if chOpts.Watch {
		resp := newResponse("message")
		resp.Body = &adminMessageBody{
			Project: pk,
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

	chID := app.channelID(pk, ch)

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
		err := app.addHistory(pk, ch, message, chOpts.HistorySize, chOpts.HistoryLifetime)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	return nil
}

// pubJoinLeave allows to publish join message into channel when
// someone subscribes on it or leave message when someone unsubscribed from channel
func (app *Application) pubJoinLeave(pk ProjectKey, ch Channel, method string, info ClientInfo) error {
	chID := app.channelID(pk, ch)
	resp := newResponse(method)
	resp.Body = &joinLeaveBody{
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
// contains information about current node
func (app *Application) pubPing() error {
	app.RLock()
	defer app.RUnlock()
	cmd := &pingControlCommand{
		Uid:      app.uid,
		Name:     app.config.Name,
		Clients:  app.nClients(),
		Unique:   app.nUniqueClients(),
		Channels: app.nChannels(),
		Started:  app.started,
	}

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
// nodes could unsubscribe user from channel
func (app *Application) pubUnsubscribe(pk ProjectKey, user UserID, ch Channel) error {

	cmd := &unsubscribeControlCommand{
		Project: pk,
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
// nodes could disconnect user from Centrifugo
func (app *Application) pubDisconnect(pk ProjectKey, user UserID) error {

	cmd := &disconnectControlCommand{
		Project: pk,
		User:    user,
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	return app.pubControl("disconnect", cmdBytes)
}

// pingCmd handles ping control command i.e. updates information about known nodes
func (app *Application) pingCmd(cmd *pingControlCommand) error {
	info := &nodeInfo{
		Uid:      cmd.Uid,
		Name:     cmd.Name,
		Clients:  cmd.Clients,
		Unique:   cmd.Unique,
		Channels: cmd.Channels,
		Started:  cmd.Started,
		Updated:  time.Now().Unix(),
	}
	app.nodesMu.Lock()
	app.nodes[cmd.Uid] = info
	app.nodesMu.Unlock()
	return nil
}

// channelID returns internal name of channel ChannelID - as
// every project can have channels with the same name we should distinguish
// between them. This also prevents collapses with admin and control
// channel names
func (app *Application) channelID(pk ProjectKey, ch Channel) ChannelID {
	app.RLock()
	defer app.RUnlock()
	return ChannelID(app.config.ChannelPrefix + "." + string(pk) + "." + string(ch))
}

// addConn registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand
func (app *Application) addConn(c clientConn) error {
	return app.connHub.add(c)
}

// removeConn removes client connection from connection registry
func (app *Application) removeConn(c clientConn) error {
	return app.connHub.remove(c)
}

// addSub registers subscription of connection on channel in both
// engine and clientSubscriptionHub
func (app *Application) addSub(pk ProjectKey, ch Channel, c clientConn) error {
	chID := app.channelID(pk, ch)
	err := app.engine.subscribe(chID)
	if err != nil {
		return err
	}
	return app.subs.add(chID, c)
}

// removeSub removes subscription of connection on channel
// from both engine and clientSubscriptionHub
func (app *Application) removeSub(pk ProjectKey, ch Channel, c clientConn) error {
	chID := app.channelID(pk, ch)
	err := app.engine.unsubscribe(chID)
	if err != nil {
		return err
	}
	return app.subs.remove(chID, c)
}

// Unsubscribe unsubscribes project user from channel, if channel is equal to empty
// string then user will be unsubscribed from all channels.
func (app *Application) Unsubscribe(pk ProjectKey, user UserID, ch Channel) error {

	if string(user) == "" {
		return ErrInvalidMessage
	}

	if string(ch) != "" {
		_, err := app.channelOpts(pk, ch)
		if err != nil {
			return err
		}
	}

	// first unsubscribe on this node
	err := app.unsubscribeUser(pk, user, ch)
	if err != nil {
		return ErrInternalServerError
	}
	// second send unsubscribe control message to other nodes
	err = app.pubUnsubscribe(pk, user, ch)
	if err != nil {
		return ErrInternalServerError
	}
	return nil
}

// unsubscribeUser unsubscribes user from channel on this node. If channel
// is an empty string then user will be unsubscribed from all channels
func (app *Application) unsubscribeUser(pk ProjectKey, user UserID, ch Channel) error {
	userConnections := app.connHub.userConnections(pk, user)
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
func (app *Application) Disconnect(pk ProjectKey, user UserID) error {

	if string(user) == "" {
		return ErrInvalidMessage
	}

	// first disconnect user from this node
	err := app.disconnectUser(pk, user)
	if err != nil {
		return ErrInternalServerError
	}
	// second send disconnect control message to other nodes
	err = app.pubDisconnect(pk, user)
	if err != nil {
		return ErrInternalServerError
	}
	return nil
}

// disconnectUser closes client connections of user on current node
func (app *Application) disconnectUser(pk ProjectKey, user UserID) error {
	userConnections := app.connHub.userConnections(pk, user)
	for _, c := range userConnections {
		err := c.close("disconnect")
		if err != nil {
			return err
		}
	}
	return nil
}

// projectByKey returns a project by project key (name) using structure
func (app *Application) projectByKey(pk ProjectKey) (Project, bool) {
	app.RLock()
	defer app.RUnlock()
	return app.structure.projectByKey(pk)
}

// namespaceKey returns namespace key from channel name if exists
func (app *Application) namespaceKey(ch Channel) NamespaceKey {
	cTrim := strings.TrimPrefix(string(ch), app.config.PrivateChannelPrefix)
	parts := strings.SplitN(cTrim, app.config.NamespaceChannelBoundary, 2)
	if len(parts) >= 2 {
		return NamespaceKey(parts[0])
	} else {
		return NamespaceKey("")
	}
}

// channelOpts returns channel options for channel using current application structure
func (app *Application) channelOpts(pk ProjectKey, ch Channel) (ChannelOptions, error) {
	app.RLock()
	defer app.RUnlock()
	nk := app.namespaceKey(ch)
	return app.structure.channelOpts(pk, nk)
}

// addPresence proxies presence adding to engine
func (app *Application) addPresence(pk ProjectKey, ch Channel, uid ConnID, info ClientInfo) error {
	chID := app.channelID(pk, ch)
	return app.engine.addPresence(chID, uid, info)
}

// removePresence proxies presence removing to engine
func (app *Application) removePresence(pk ProjectKey, ch Channel, uid ConnID) error {
	chID := app.channelID(pk, ch)
	return app.engine.removePresence(chID, uid)
}

// Presence returns a map of active clients in project channel.
func (app *Application) Presence(pk ProjectKey, ch Channel) (map[ConnID]ClientInfo, error) {

	if string(ch) == "" {
		return map[ConnID]ClientInfo{}, ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(pk, ch)
	if err != nil {
		return map[ConnID]ClientInfo{}, err
	}

	if !chOpts.Presence {
		return map[ConnID]ClientInfo{}, ErrNotAvailable
	}

	chID := app.channelID(pk, ch)

	presence, err := app.engine.presence(chID)
	if err != nil {
		return map[ConnID]ClientInfo{}, ErrInternalServerError
	}
	return presence, nil
}

// addHistory proxies history message adding to engine
func (app *Application) addHistory(pk ProjectKey, ch Channel, message Message, size, lifetime int64) error {
	chID := app.channelID(pk, ch)
	return app.engine.addHistoryMessage(chID, message, size, lifetime)
}

// History returns a slice of last messages published into project channel.
func (app *Application) History(pk ProjectKey, ch Channel) ([]Message, error) {

	if string(ch) == "" {
		return []Message{}, ErrInvalidMessage
	}

	chOpts, err := app.channelOpts(pk, ch)
	if err != nil {
		return []Message{}, err
	}

	if chOpts.HistorySize <= 0 || chOpts.HistoryLifetime <= 0 {
		return []Message{}, ErrNotAvailable
	}

	chID := app.channelID(pk, ch)

	history, err := app.engine.history(chID)
	if err != nil {
		return []Message{}, ErrInternalServerError
	}
	return history, nil
}

// privateChannel checks if channel private and therefore subscription
// request on it must be properly signed on web application backend
func (app *Application) privateChannel(ch Channel) bool {
	app.RLock()
	defer app.RUnlock()
	return strings.HasPrefix(string(ch), app.config.PrivateChannelPrefix)
}

// userAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it
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

// addAdminConn registers an admin connection in adminConnectionHub
func (app *Application) addAdminConn(c adminConn) error {
	return app.admins.add(c)
}

// removeAdminConn admin connection from adminConnectionHub
func (app *Application) removeAdminConn(c adminConn) error {
	return app.admins.remove(c)
}

// nChannels returns total amount of active channels on this node
func (app *Application) nChannels() int {
	return app.subs.nChannels()
}

// nClients returns total amount of client connections to this node
func (app *Application) nClients() int {
	return app.connHub.nClients()
}

// nUniqueClients returns total amount of unique client
// connections to this node
func (app *Application) nUniqueClients() int {
	return app.connHub.nUniqueClients()
}

const (
	AuthTokenKey   = "token"
	AuthTokenValue = "authorized"
)

// checkAuthToken checks admin connection token which Centrifugo returns after admin login
func (app *Application) checkAuthToken(token string) error {

	app.RLock()
	secret := app.config.WebSecret
	app.RUnlock()

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
