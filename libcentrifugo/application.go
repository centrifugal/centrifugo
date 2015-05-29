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
	// nodesMutex allows to synchronize access to nodes
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
	Name     UserID `json:"name"`
	Clients  int    `json:"clients"`
	Unique   int    `json:"unique"`
	Channels int    `json:"channels"`
	Started  int64  `json:"started"`
	Updated  int64  `json:"-"`
}

func newApplication() (*application, error) {
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

// initialize used to set configuration dependent application properties
func (app *application) initialize() {
	app.Lock()
	defer app.Unlock()
	app.config = newConfig()
	app.structure = structureFromConfig(nil)
	if app.config.insecure {
		logger.WARN.Println("application initialized in INSECURE MODE")
	}
	if app.structure.ProjectList == nil {
		logger.FATAL.Println("project structure not found, please configure at least one project")
	}
}

// setEngine binds engine to application
func (app *application) setEngine(e engine) {
	app.Lock()
	defer app.Unlock()
	app.engine = e
}

// handleMsg called when new message of any type received by this node.
// It looks at channel and decides which message handler to call
func (app *application) handleMsg(channel string, message []byte) error {
	switch channel {
	case app.config.controlChannel:
		return app.controlMsg(message)
	case app.config.adminChannel:
		return app.adminMsg(message)
	default:
		return app.clientMsg(channel, message)
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
		err := json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidControlMessage
		}
		return app.pingCmd(&cmd)
	case "unsubscribe":
		var cmd unsubscribeControlCommand
		err := json.Unmarshal(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidControlMessage
		}
		return app.unsubscribeCmd(&cmd)
	case "disconnect":
		var cmd disconnectControlCommand
		err := json.Unmarshal(params, &cmd)
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
func (app *application) clientMsg(channel string, message []byte) error {
	return app.subs.broadcast(channel, string(message))
}

// pubControl publishes message into control channel so all running
// nodes will receive and handle it
func (app *application) pubControl(message []byte) error {
	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.controlChannel, message)
}

// pubAdmin publishes message into admin channel so all running
// nodes will receive it and send to admins connected
func (app *application) pubAdmin(message []byte) error {
	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.adminChannel, message)
}

type Message struct {
	Uid       string           `json:"uid"`
	Timestamp string           `json:"timestamp"`
	Info      *ClientInfo      `json:"info"`
	Channel   ChannelID        `json:"channel"`
	Data      *json.RawMessage `json:"data"`
}

// pubClient publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel
func (app *application) pubClient(p *project, channel ChannelID, chOpts *ChannelOptions, data []byte, info *ClientInfo) error {

	uid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	raw := json.RawMessage(data)

	message := Message{
		Uid:       uid.String(),
		Timestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Info:      info,
		Channel:   channel,
		Data:      &raw,
	}

	if chOpts.Watch {
		resp := newResponse("message")
		resp.Body = map[string]interface{}{
			"project": p.Name,
			"message": message,
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

	projectChannel := app.projectChannel(p.Name, channel)

	resp := newResponse("message")
	resp.Body = message

	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	err = app.engine.publish(projectChannel, byteMessage)
	if err != nil {
		return err
	}

	if chOpts.HistorySize > 0 && chOpts.HistoryLifetime > 0 {
		err := app.addHistory(p.Name, channel, message, chOpts.HistorySize, chOpts.HistoryLifetime)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	return nil
}

// pubJoinLeave allows to publish join message into channel when
// someone subscribes on it or leave message when someone unsubscribed from channel
func (app *application) pubJoinLeave(projectKey ProjectKey, channel ChannelID, method string, info ClientInfo) error {
	projectChannel := app.projectChannel(projectKey, channel)
	resp := newResponse(method)
	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    info,
	}
	byteMessage, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return app.engine.publish(projectChannel, byteMessage)
}

func (app *application) pubPing() error {
	app.RLock()
	defer app.RUnlock()
	cmd := &pingControlCommand{
		Uid:      app.uid,
		Name:     UserID(app.config.name),
		Clients:  app.nClients(),
		Unique:   app.nUniqueClients(),
		Channels: app.nChannels(),
		Started:  app.started,
	}

	err := app.pingCmd(cmd)
	if err != nil {
		logger.ERROR.Println(err)
	}

	message := map[string]interface{}{
		"uid":    app.uid,
		"method": "ping",
		"params": cmd,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return app.pubControl(messageBytes)
}

func (app *application) pubUnsub(projectKey ProjectKey, user UserID, channel ChannelID) error {
	message := map[string]interface{}{
		"uid":    app.uid,
		"method": "unsubscribe",
		"params": map[string]interface{}{
			"project": projectKey,
			"user":    user,
			"channel": channel,
		},
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return app.pubControl(messageBytes)
}

func (app *application) pubDisconnect(projectKey ProjectKey, user UserID) error {
	message := map[string]interface{}{
		"uid":    app.uid,
		"method": "disconnect",
		"params": map[string]interface{}{
			"project": projectKey,
			"user":    user,
		},
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return app.pubControl(messageBytes)
}

// pingCmd updates information about known nodes
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

func (app *application) unsubscribeCmd(cmd *unsubscribeControlCommand) error {
	return app.unsubUser(cmd.Project, cmd.User, cmd.Channel)
}

/*
func (app *application) disconnectCmd(cmd *disconnectControlCommand) error {
	return app.disconnectUser(cmd.Project, cmd.User)
}
*/
// projectChannel returns internal name of channel - as
// every project can have channels with the same name we should distinguish
// between them. This also prevents collapses with admin and control
// channel names
func (app *application) projectChannel(projectKey ProjectKey, channel ChannelID) string {
	app.RLock()
	defer app.RUnlock()
	return app.config.channelPrefix + "." + string(projectKey) + "." + string(channel)
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
func (app *application) addSub(projectKey ProjectKey, channel ChannelID, c clientConn) error {
	projectChannel := app.projectChannel(projectKey, channel)
	err := app.engine.subscribe(projectChannel)
	if err != nil {
		return err
	}
	return app.subs.add(projectChannel, c)
}

// removeSub removes subscription of connection on channel
// from both engine and clientSubscriptionHub
func (app *application) removeSub(projectKey ProjectKey, channel ChannelID, c clientConn) error {
	projectChannel := app.projectChannel(projectKey, channel)
	err := app.engine.unsubscribe(projectChannel)
	if err != nil {
		return err
	}
	return app.subs.remove(projectChannel, c)
}

// unsubUser unsubscribes user from channel on this node. If channel
// is an empty string then user will be unsubscribed from all channels
func (app *application) unsubUser(projectKey ProjectKey, user UserID, channel ChannelID) error {
	userConnections := app.connHub.userConnections(projectKey, user)
	for _, c := range userConnections {
		var channels []ChannelID
		if channel == "" {
			// unsubscribe from all channels
			channels = c.channels()
		} else {
			channels = []ChannelID{channel}
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
func (app *application) disconnectUser(projectKey ProjectKey, user UserID) error {
	userConnections := app.connHub.userConnections(projectKey, user)
	for _, c := range userConnections {
		err := c.close("disconnect")
		if err != nil {
			return err
		}
	}
	return nil
}

// projectByKey returns a project by project key (name) using structure
func (app *application) projectByKey(projectKey ProjectKey) (*project, bool) {
	app.RLock()
	defer app.RUnlock()
	return app.structure.projectByKey(projectKey)
}

// channelNamespace returns namespace name from channel name if exists or
// empty string
func (app *application) channelNamespace(channel ChannelID) NamespaceKey {
	cTrim := strings.TrimPrefix(string(channel), app.config.privateChannelPrefix)
	parts := strings.SplitN(cTrim, app.config.namespaceChannelBoundary, 2)
	if len(parts) >= 2 {
		return NamespaceKey(parts[0])
	} else {
		return ""
	}
}

// channelOpts returns channel options for channel using structure
func (app *application) channelOpts(p ProjectKey, c ChannelID) *ChannelOptions {
	app.RLock()
	defer app.RUnlock()
	namespaceName := app.channelNamespace(c)
	return app.structure.channelOpts(p, namespaceName)
}

// addPresence proxies presence adding to engine
func (app *application) addPresence(projectKey ProjectKey, channel ChannelID, uid string, info ClientInfo) error {
	projectChannel := app.projectChannel(projectKey, channel)
	return app.engine.addPresence(projectChannel, uid, info)
}

// removePresence proxies presence removing to engine
func (app *application) removePresence(projectKey ProjectKey, channel ChannelID, uid string) error {
	projectChannel := app.projectChannel(projectKey, channel)
	return app.engine.removePresence(projectChannel, uid)
}

// getPresence proxies presence extraction to engine
func (app *application) presence(projectKey ProjectKey, channel ChannelID) (map[string]ClientInfo, error) {
	projectChannel := app.projectChannel(projectKey, channel)
	return app.engine.presence(projectChannel)
}

// addHistory proxies history message adding to engine
func (app *application) addHistory(projectKey ProjectKey, channel ChannelID, message Message, size, lifetime int64) error {
	projectChannel := app.projectChannel(projectKey, channel)
	return app.engine.addHistoryMessage(projectChannel, message, size, lifetime)
}

// getHistory proxies history extraction to engine
func (app *application) history(projectKey ProjectKey, channel ChannelID) ([]Message, error) {
	projectChannel := app.projectChannel(projectKey, channel)
	return app.engine.history(projectChannel)
}

// privateCh checks if channel private and therefore subscription
// request on it must be properly signed on web application backend
func (app *application) privateChannel(channel ChannelID) bool {
	app.RLock()
	defer app.RUnlock()
	return strings.HasPrefix(string(channel), app.config.privateChannelPrefix)
}

// userAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it
func (app *application) userAllowed(channel ChannelID, user UserID) bool {
	app.RLock()
	defer app.RUnlock()
	if !strings.Contains(string(channel), app.config.userChannelBoundary) {
		return true
	}
	parts := strings.Split(string(channel), app.config.userChannelBoundary)
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

func (app *application) checkAuthToken(token string) error {

	app.RLock()
	secret := app.config.webSecret
	app.RUnlock()

	if secret == "" {
		logger.ERROR.Println("provide secret in configuration")
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
