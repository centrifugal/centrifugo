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
	"github.com/mitchellh/mapstructure"
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
	nodesMutex sync.Mutex

	// hub to manage client connections
	clientConnectionHub *clientConnectionHub
	// hub to manage client subscriptions
	clientSubscriptionHub *clientSubscriptionHub
	// hub to manage admin connections
	adminConnectionHub *adminConnectionHub

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

func newApplication() (*application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	app := &application{
		uid:                   uid.String(),
		nodes:                 make(map[string]*nodeInfo),
		clientConnectionHub:   newClientConnectionHub(),
		clientSubscriptionHub: newClientSubscriptionHub(),
		adminConnectionHub:    newAdminConnectionHub(),
		started:               time.Now().Unix(),
	}
	return app, nil
}

func (app *application) run() {
	go app.sendPingMessage()
	go app.cleanNodeInfo()
}

func (app *application) sendPingMessage() {
	for {
		err := app.publishPingControlMessage()
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

		app.nodesMutex.Lock()
		for uid, info := range app.nodes {
			if time.Now().Unix()-info.Updated > delay {
				delete(app.nodes, uid)
			}
		}
		app.nodesMutex.Unlock()

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

// handleMessage called when new message of any type received by this node.
// It looks at channel and decides which message handler to call
func (app *application) handleMessage(channel string, message []byte) error {
	switch channel {
	case app.config.controlChannel:
		return app.handleControlMessage(message)
	case app.config.adminChannel:
		return app.handleAdminMessage(message)
	default:
		return app.handleClientMessage(channel, message)
	}
}

// handleControlMessage handles messages from control channel - control
// messages used for internal communication between nodes to share state
// or commands
func (app *application) handleControlMessage(message []byte) error {

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
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidControlMessage
		}
		return app.handlePingControlCommand(&cmd)
	case "unsubscribe":
		var cmd unsubscribeControlCommand
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidControlMessage
		}
		return app.handleUnsubscribeControlCommand(&cmd)
	case "disconnect":
		var cmd disconnectControlCommand
		err := mapstructure.Decode(params, &cmd)
		if err != nil {
			logger.ERROR.Println(err)
			return ErrInvalidControlMessage
		}
		return app.handleDisconnectControlCommand(&cmd)
	default:
		logger.ERROR.Println("unknown control message method", method)
		return ErrInvalidControlMessage
	}

	return nil
}

// handleAdminMessage handles messages from admin channel - those messages
// must be delivered to all admins connected to this node
func (app *application) handleAdminMessage(message []byte) error {
	return app.adminConnectionHub.broadcast(string(message))
}

// handleClientMessage handles messages published by web application or client
// into channel. The goal of this method to deliver this message to all clients
// on this node subscribed on channel
func (app *application) handleClientMessage(channel string, message []byte) error {
	return app.clientSubscriptionHub.broadcast(channel, string(message))
}

// publishControlMessage publishes message into control channel so all running
// nodes will receive and handle it
func (app *application) publishControlMessage(message []byte) error {
	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.controlChannel, message)
}

// publishAdminMessage publishes message into admin channel so all running
// nodes will receive it and send to admins connected
func (app *application) publishAdminMessage(message []byte) error {
	app.RLock()
	defer app.RUnlock()
	return app.engine.publish(app.config.adminChannel, message)
}

type Message struct {
	Uid       string      `json:"uid"`
	Timestamp string      `json:"timestamp"`
	Info      *ClientInfo `json:"info"`
	Channel   string      `json:"channel"`
	Data      interface{} `json:"data"`
}

// publishClientMessage publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel
func (app *application) publishClientMessage(p *project, channel string, chOpts *ChannelOptions, data interface{}, info *ClientInfo) error {

	uid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	message := Message{
		Uid:       uid.String(),
		Timestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Info:      info,
		Channel:   channel,
		Data:      data,
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
			err = app.publishAdminMessage(messageBytes)
			if err != nil {
				logger.ERROR.Println(err)
			}
		}
	}

	projectChannel := app.getProjectChannel(p.Name, channel)

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
		err := app.addHistoryMessage(p.Name, channel, message, chOpts.HistorySize, chOpts.HistoryLifetime)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	return nil
}

// publishJoinLeaveMessage allows to publish join message into channel when
// someone subscribes on it or leave message when someone unsubscribed from channel
func (app *application) publishJoinLeaveMessage(projectKey, channel, method string, info ClientInfo) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
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

func (app *application) publishPingControlMessage() error {
	app.RLock()
	defer app.RUnlock()
	cmd := &pingControlCommand{
		Uid:      app.uid,
		Name:     app.config.name,
		Clients:  app.getClientsCount(),
		Unique:   app.getUniqueChannelsCount(),
		Channels: app.getChannelsCount(),
		Started:  app.started,
	}

	err := app.handlePingControlCommand(cmd)
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
	return app.publishControlMessage(messageBytes)
}

func (app *application) publishUnsubscribeControlMessage(projectKey, user, channel string) error {
	message := map[string]interface{}{
		"uid":    app.uid,
		"method": "unsubscribe",
		"params": map[string]string{
			"project": projectKey,
			"user":    user,
			"channel": channel,
		},
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return app.publishControlMessage(messageBytes)
}

func (app *application) publishDisconnectControlMessage(projectKey, user string) error {
	message := map[string]interface{}{
		"uid":    app.uid,
		"method": "disconnect",
		"params": map[string]string{
			"project": projectKey,
			"user":    user,
		},
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return app.publishControlMessage(messageBytes)
}

// handlePingControlCommand updates information about known nodes
func (app *application) handlePingControlCommand(cmd *pingControlCommand) error {
	info := &nodeInfo{
		Uid:      cmd.Uid,
		Name:     cmd.Name,
		Clients:  cmd.Clients,
		Unique:   cmd.Unique,
		Channels: cmd.Channels,
		Started:  cmd.Started,
		Updated:  time.Now().Unix(),
	}
	app.nodesMutex.Lock()
	app.nodes[cmd.Uid] = info
	app.nodesMutex.Unlock()
	return nil
}

func (app *application) handleUnsubscribeControlCommand(cmd *unsubscribeControlCommand) error {
	return app.unsubscribeUserFromChannel(cmd.Project, cmd.User, cmd.Channel)
}

func (app *application) handleDisconnectControlCommand(cmd *disconnectControlCommand) error {
	return app.disconnectUser(cmd.Project, cmd.User)
}

// getProjectChannel returns internal name of channel - as
// every project can have channels with the same name we should distinguish
// between them. This also prevents collapses with admin and control
// channel names
func (app *application) getProjectChannel(projectKey, channel string) string {
	app.RLock()
	defer app.RUnlock()
	return app.config.channelPrefix + "." + projectKey + "." + channel
}

// addConnection registers authenticated connection in clientConnectionHub
// this allows to make operations with user connection on demand
func (app *application) addConnection(c clientConnection) error {
	return app.clientConnectionHub.add(c)
}

// removeConnection removes client connection from connection registry
func (app *application) removeConnection(c clientConnection) error {
	return app.clientConnectionHub.remove(c)
}

// addSubscription registers subscription of connection on channel in both
// engine and clientSubscriptionHub
func (app *application) addSubscription(projectKey, channel string, c clientConnection) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	err := app.engine.subscribe(projectChannel)
	if err != nil {
		return err
	}
	return app.clientSubscriptionHub.add(projectChannel, c)
}

// removeSubscription removes subscription of connection on channel
// from both engine and clientSubscriptionHub
func (app *application) removeSubscription(projectKey, channel string, c clientConnection) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	err := app.engine.unsubscribe(projectChannel)
	if err != nil {
		return err
	}
	return app.clientSubscriptionHub.remove(projectChannel, c)
}

// unsubscribeUserFromChannel unsubscribes user from channel on this node. If channel
// is an empty string then user will be unsubscribed from all channels
func (app *application) unsubscribeUserFromChannel(projectKey, user, channel string) error {
	userConnections := app.clientConnectionHub.getUserConnections(projectKey, user)
	for _, c := range userConnections {
		var channels []string
		if channel == "" {
			// unsubscribe from all channels
			channels = c.getChannels()
		} else {
			channels = []string{channel}
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
func (app *application) disconnectUser(projectKey, user string) error {
	userConnections := app.clientConnectionHub.getUserConnections(projectKey, user)
	for _, c := range userConnections {
		err := c.close("disconnect")
		if err != nil {
			return err
		}
	}
	return nil
}

// getProjectByKey returns a project by project key (name) using structure
func (app *application) getProjectByKey(projectKey string) (*project, bool) {
	app.RLock()
	defer app.RUnlock()
	return app.structure.getProjectByKey(projectKey)
}

// extractNamespaceName returns namespace name from channel name if exists or
// empty string
func (app *application) extractNamespaceName(channel string) string {
	channel = strings.TrimPrefix(channel, app.config.privateChannelPrefix)
	parts := strings.SplitN(channel, app.config.namespaceChannelBoundary, 2)
	if len(parts) >= 2 {
		return parts[0]
	} else {
		return ""
	}
}

// getChannelOptions returns channel options for channel using structure
func (app *application) getChannelOptions(projectKey, channel string) *ChannelOptions {
	app.RLock()
	defer app.RUnlock()
	namespaceName := app.extractNamespaceName(channel)
	return app.structure.getChannelOptions(projectKey, namespaceName)
}

// addPresence proxies presence adding to engine
func (app *application) addPresence(projectKey, channel, uid string, info ClientInfo) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.addPresence(projectChannel, uid, info)
}

// removePresence proxies presence removing to engine
func (app *application) removePresence(projectKey, channel, uid string) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.removePresence(projectChannel, uid)
}

// getPresence proxies presence extraction to engine
func (app *application) getPresence(projectKey, channel string) (map[string]interface{}, error) {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.getPresence(projectChannel)
}

// addHistoryMessage proxies history message adding to engine
func (app *application) addHistoryMessage(projectKey, channel string, message Message, size, lifetime int64) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.addHistoryMessage(projectChannel, message, size, lifetime)
}

// getHistory proxies history extraction to engine
func (app *application) getHistory(projectKey, channel string) ([]Message, error) {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.getHistory(projectChannel)
}

// isPrivateChannel checks if channel private and therefore subscription
// request on it must be properly signed on web application backend
func (app *application) isPrivateChannel(channel string) bool {
	app.RLock()
	defer app.RUnlock()
	return strings.HasPrefix(channel, app.config.privateChannelPrefix)
}

// isUserAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it
func (app *application) isUserAllowed(channel, user string) bool {
	app.RLock()
	defer app.RUnlock()
	if !strings.Contains(channel, app.config.userChannelBoundary) {
		return true
	}
	parts := strings.Split(channel, app.config.userChannelBoundary)
	allowedUsers := strings.Split(parts[len(parts)-1], app.config.userChannelSeparator)
	for _, allowedUser := range allowedUsers {
		if user == allowedUser {
			return true
		}
	}
	return false
}

// register admin connection in adminConnectionHub
func (app *application) addAdminConnection(c adminConnection) error {
	return app.adminConnectionHub.add(c)
}

// unregister admin connection from adminConnectionHub
func (app *application) removeAdminConnection(c adminConnection) error {
	return app.adminConnectionHub.remove(c)
}

// getChannelsCount returns total amount of active channels on this node
func (app *application) getChannelsCount() int {
	return app.clientSubscriptionHub.getChannelsCount()
}

// getClientsCount returns total amount of client connections to this node
func (app *application) getClientsCount() int {
	return app.clientConnectionHub.getClientsCount()
}

// getUniqueChannelsCount returns total amount of unique client
// connections to this node
func (app *application) getUniqueChannelsCount() int {
	return app.clientConnectionHub.getUniqueClientsCount()
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
