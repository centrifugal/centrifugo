package libcentrifugo

// TODO: maybe use interfaces instead of app reference in client and engine?

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

type application struct {
	sync.RWMutex

	// unique id for this application (node)
	uid string

	// started is unix time of node start
	started int64

	// nodes is a map with information about nodes known
	nodes map[string]*nodeInfo
	// nodesMu allows to synchronize access to nodes
	nodesMu sync.Mutex

	// hub to manage client connections
	connHub *clientHub
	// hub to manage client subscriptions
	subs *subHub
	// hub to manage admin connections
	admins *adminHub

	// engine to use - in memory or redis
	engine engine

	// reference to structure to work with projects and namespaces
	structure *structure

	// config for application
	config *config
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

func newApplication(c *config) (*application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	app := &application{
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

func (app *application) run() {
	go app.sendPingMsg()
	go app.cleanNodeInfo()
}

func (app *application) sendPingMsg() {
	for {
		err := app.pubPing()
		if err != nil {
			logger.CRITICAL.Println(err)
		}
		app.RLock()
		interval := app.config.nodePingInterval
		app.RUnlock()
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func (app *application) cleanNodeInfo() {
	for {
		app.RLock()
		delay := app.config.nodeInfoMaxDelay
		app.RUnlock()

		app.nodesMu.Lock()
		for uid, info := range app.nodes {
			if time.Now().Unix()-info.Updated > delay {
				delete(app.nodes, uid)
			}
		}
		app.nodesMu.Unlock()

		app.RLock()
		interval := app.config.nodeInfoCleanInterval
		app.RUnlock()

		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// initialize used to make various actions after application instance fully configured
func (app *application) initialize() {
	app.RLock()
	defer app.RUnlock()
	if app.config.insecure {
		logger.WARN.Println("application initialized in INSECURE MODE")
	}
	if app.structure.ProjectList == nil {
		logger.FATAL.Println("project structure not found, please configure at least one project")
	}
}

// setConfig binds config to application
func (app *application) setConfig(c *config) {
	app.Lock()
	defer app.Unlock()
	app.config = c
}

// setEngine binds structure to application
func (app *application) setStructure(s *structure) {
	app.Lock()
	defer app.Unlock()
	app.structure = s
}

// setEngine binds engine to application
func (app *application) setEngine(e engine) {
	app.Lock()
	defer app.Unlock()
	app.engine = e
}

// handleMsg called when new message of any type received by this node.
// It looks at channel and decides which message handler to call
func (app *application) handleMsg(chID ChannelID, message []byte) error {
	switch chID {
	case app.config.controlChannel:
		return app.controlMsg(message)
	case app.config.adminChannel:
		return app.adminMsg(message)
	default:
		return app.clientMsg(chID, message)
	}
}

// controlMsg handles messages from control channel - control
// messages used for internal communication between nodes to share state
// or commands
func (app *application) controlMsg(message []byte) error {

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
			return ErrInvalidControlMessage
		}
		return app.pingCmd(&cmd)
	case "unsubscribe":
		var cmd unsubscribeControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidControlMessage
		}
		return app.unsubscribeUser(cmd.Project, cmd.User, cmd.Channel)
	case "disconnect":
		var cmd disconnectControlCommand
		err := json.Unmarshal(*params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidControlMessage
		}
		return app.disconnectUser(cmd.Project, cmd.User)
	default:
		logger.ERROR.Println("unknown control message method", method)
		return ErrInvalidControlMessage
	}
	panic("unreachable")
}

// adminMsg handles messages from admin channel - those messages
// must be delivered to all admins connected to this node
func (app *application) adminMsg(message []byte) error {
	return app.admins.broadcast(string(message))
}

// clientMsg handles messages published by web application or client
// into channel. The goal of this method to deliver this message to all clients
// on this node subscribed on channel
func (app *application) clientMsg(chID ChannelID, message []byte) error {
	return app.subs.broadcast(chID, string(message))
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it
func (app *application) pubControl(method string, params []byte) error {

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
	return app.engine.publish(app.config.controlChannel, messageBytes)
}

// pubAdmin publishes message into admin channel so all running
// nodes will receive it and send to admins connected
func (app *application) pubAdmin(message []byte) error {
	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.adminChannel, message)
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

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel
func (app *application) pubClient(p Project, ch Channel, chOpts ChannelOptions, data []byte, client ConnID, info *ClientInfo) error {

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
			Project: p.Name,
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

	chID := app.channelID(p.Name, ch)

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
		err := app.addHistory(p.Name, ch, message, chOpts.HistorySize, chOpts.HistoryLifetime)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	return nil
}

// pubJoinLeave allows to publish join message into channel when
// someone subscribes on it or leave message when someone unsubscribed from channel
func (app *application) pubJoinLeave(pk ProjectKey, ch Channel, method string, info ClientInfo) error {
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
func (app *application) pubPing() error {
	app.RLock()
	defer app.RUnlock()
	cmd := &pingControlCommand{
		Uid:      app.uid,
		Name:     app.config.name,
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
func (app *application) pubUnsubscribe(pk ProjectKey, user UserID, ch Channel) error {

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
func (app *application) pubDisconnect(pk ProjectKey, user UserID) error {

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
func (app *application) pingCmd(cmd *pingControlCommand) error {
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
func (app *application) channelID(pk ProjectKey, ch Channel) ChannelID {
	app.RLock()
	defer app.RUnlock()
	return ChannelID(app.config.channelPrefix + "." + string(pk) + "." + string(ch))
}

// addConn registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand
func (app *application) addConn(c clientConn) error {
	return app.connHub.add(c)
}

// removeConn removes client connection from connection registry
func (app *application) removeConn(c clientConn) error {
	return app.connHub.remove(c)
}

// addSub registers subscription of connection on channel in both
// engine and clientSubscriptionHub
func (app *application) addSub(pk ProjectKey, ch Channel, c clientConn) error {
	chID := app.channelID(pk, ch)
	err := app.engine.subscribe(chID)
	if err != nil {
		return err
	}
	return app.subs.add(chID, c)
}

// removeSub removes subscription of connection on channel
// from both engine and clientSubscriptionHub
func (app *application) removeSub(pk ProjectKey, ch Channel, c clientConn) error {
	chID := app.channelID(pk, ch)
	err := app.engine.unsubscribe(chID)
	if err != nil {
		return err
	}
	return app.subs.remove(chID, c)
}

// unsubscribeUser unsubscribes user from channel on this node. If channel
// is an empty string then user will be unsubscribed from all channels
func (app *application) unsubscribeUser(pk ProjectKey, user UserID, channel Channel) error {
	userConnections := app.connHub.userConnections(pk, user)
	for _, c := range userConnections {
		var channels []Channel
		if channel == "" {
			// unsubscribe from all channels
			channels = c.channels()
		} else {
			channels = []Channel{channel}
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

// disconnectUser closes client connections of user
func (app *application) disconnectUser(pk ProjectKey, user UserID) error {
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
func (app *application) projectByKey(pk ProjectKey) (Project, bool) {
	app.RLock()
	defer app.RUnlock()
	return app.structure.projectByKey(pk)
}

// namespaceKey returns namespace key from channel name if exists
func (app *application) namespaceKey(ch Channel) NamespaceKey {
	cTrim := strings.TrimPrefix(string(ch), app.config.privateChannelPrefix)
	parts := strings.SplitN(cTrim, app.config.namespaceChannelBoundary, 2)
	if len(parts) >= 2 {
		return NamespaceKey(parts[0])
	} else {
		return NamespaceKey("")
	}
}

// channelOpts returns channel options for channel using structure
func (app *application) channelOpts(pk ProjectKey, ch Channel) (ChannelOptions, error) {
	app.RLock()
	defer app.RUnlock()
	nk := app.namespaceKey(ch)
	return app.structure.channelOpts(pk, nk)
}

// addPresence proxies presence adding to engine
func (app *application) addPresence(pk ProjectKey, ch Channel, uid ConnID, info ClientInfo) error {
	chID := app.channelID(pk, ch)
	return app.engine.addPresence(chID, uid, info)
}

// removePresence proxies presence removing to engine
func (app *application) removePresence(pk ProjectKey, ch Channel, uid ConnID) error {
	chID := app.channelID(pk, ch)
	return app.engine.removePresence(chID, uid)
}

// presence proxies presence extraction to engine
func (app *application) presence(pk ProjectKey, ch Channel) (map[ConnID]ClientInfo, error) {
	chID := app.channelID(pk, ch)
	return app.engine.presence(chID)
}

// addHistory proxies history message adding to engine
func (app *application) addHistory(pk ProjectKey, ch Channel, message Message, size, lifetime int64) error {
	chID := app.channelID(pk, ch)
	return app.engine.addHistoryMessage(chID, message, size, lifetime)
}

// history proxies history extraction to engine
func (app *application) history(pk ProjectKey, ch Channel) ([]Message, error) {
	chID := app.channelID(pk, ch)
	return app.engine.history(chID)
}

// privateChannel checks if channel private and therefore subscription
// request on it must be properly signed on web application backend
func (app *application) privateChannel(ch Channel) bool {
	app.RLock()
	defer app.RUnlock()
	return strings.HasPrefix(string(ch), app.config.privateChannelPrefix)
}

// userAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it
func (app *application) userAllowed(ch Channel, user UserID) bool {
	app.RLock()
	defer app.RUnlock()
	if !strings.Contains(string(ch), app.config.userChannelBoundary) {
		return true
	}
	parts := strings.Split(string(ch), app.config.userChannelBoundary)
	allowedUsers := strings.Split(parts[len(parts)-1], app.config.userChannelSeparator)
	for _, allowedUser := range allowedUsers {
		if string(user) == allowedUser {
			return true
		}
	}
	return false
}

// addAdminConn registers an admin connection in adminConnectionHub
func (app *application) addAdminConn(c adminConn) error {
	return app.admins.add(c)
}

// removeAdminConn admin connection from adminConnectionHub
func (app *application) removeAdminConn(c adminConn) error {
	return app.admins.remove(c)
}

// nChannels returns total amount of active channels on this node
func (app *application) nChannels() int {
	return app.subs.nChannels()
}

// nClients returns total amount of client connections to this node
func (app *application) nClients() int {
	return app.connHub.nClients()
}

// nUniqueClients returns total amount of unique client
// connections to this node
func (app *application) nUniqueClients() int {
	return app.connHub.nUniqueClients()
}

const (
	AuthTokenKey   = "token"
	AuthTokenValue = "authorized"
)

// checkAuthToken checks admin connection token which Centrifugo returns after admin login
func (app *application) checkAuthToken(token string) error {

	app.RLock()
	secret := app.config.webSecret
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
