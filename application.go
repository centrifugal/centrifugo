package main

// TODO: maybe use interfaces instead of app reference in client and engine?

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/logger"
	"github.com/mitchellh/mapstructure"
	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
)

type application struct {
	sync.Mutex

	// unique id for this application (node)
	uid string

	// nodes is a map with information about nodes known
	nodes map[string]*nodeInfo

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

	// name of this node - provided explicitly by configuration option
	// or constructed from hostname and port
	name string
	// admin password
	password string
	// secret key to generate auth token for admin
	secret string

	// prefix before each channel
	channelPrefix string
	// channel name for admin messages
	adminChannel string
	// channel name for internal control messages between nodes
	controlChannel string

	// in seconds, how often node must send ping control message
	nodePingInterval int64
	// in seconds, how often node must clean information about other running nodes
	nodeInfoCleanInterval int64
	// in seconds, how many seconds node info considered actual
	nodeInfoMaxDelay int64
	// in seconds, how often to publish node info into admin channel
	nodeInfoPublishInterval int64

	// in seconds, how often connected clients must update presence info
	presencePingInterval int64
	// in seconds, how long to consider presence info valid after receiving presence ping
	presenceExpireInterval int64

	// in seconds, an interval given to client to refresh its connection in the end of
	// connection lifetime
	expiredConnectionCloseDelay int64

	// prefix in channel name which indicates that channel is private
	privateChannelPrefix string
	// string separator which must be put after namespace part in channel name
	namespaceChannelBoundary string
	// string separator which must be set before allowed users part in channel name
	userChannelBoundary string
	// separates allowed users in user part of channel name
	userChannelSeparator string

	// insecure turns on insecure mode - when it's turned on then no authentication
	// required at all when connecting to Centrifuge, anonymous access and publish
	// allowed for all channels, no connection check performed. This can be suitable
	// for demonstration or personal usage
	insecure bool
}

type nodeInfo struct {
	Uid       string `json:"uid"`
	Name      string `json:"name"`
	UpdatedAt int64  `json:"-"`
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
	}
	return app, nil
}

func (app *application) run() {
	go app.sendPingMessage()
	go app.cleanNodeInfo()
	go app.publishNodeInfo()
}

func (app *application) sendPingMessage() {
	for {
		err := app.publishPingControlMessage()
		if err != nil {
			logger.CRITICAL.Println(err)
		}
		time.Sleep(time.Duration(app.nodePingInterval) * time.Second)
	}
}

func (app *application) cleanNodeInfo() {
	for {
		for uid, info := range app.nodes {
			if time.Now().Unix()-info.UpdatedAt > int64(app.nodeInfoMaxDelay) {
				delete(app.nodes, uid)
			}
		}
		time.Sleep(time.Duration(app.nodeInfoCleanInterval) * time.Second)
	}
}

func (app *application) publishNodeInfo() {
	for {
		message := map[string]interface{}{
			"method": "node",
			"body": map[string]interface{}{
				"uid":     app.uid,
				"name":    app.name,
				"nodes":   len(app.nodes) + 1,
				"metrics": map[string]interface{}{},
			},
		}
		messageJson, _ := json.Marshal(message)
		err := app.publishAdminMessage(messageJson)
		if err != nil {
			logger.ERROR.Println(err)
		}
		time.Sleep(time.Duration(app.nodeInfoPublishInterval) * time.Second)
	}
}

// getApplicationName returns a name for this node. If no name provided
// in configuration then it constructs node name based on hostname and port
func getApplicationName() string {
	name := viper.GetString("name")
	if name != "" {
		return name
	}
	port := viper.GetString("port")
	var hostname string
	hostname, err := os.Hostname()
	if err != nil {
		logger.ERROR.Println(err)
		hostname = "?"
	}
	return hostname + "_" + port
}

// initialize used to set configuration dependent application properties
func (app *application) initialize() {
	app.Lock()
	defer app.Unlock()
	app.password = viper.GetString("password")
	app.secret = viper.GetString("secret")
	app.channelPrefix = viper.GetString("channel_prefix")
	app.adminChannel = app.channelPrefix + "." + "admin"
	app.controlChannel = app.channelPrefix + "." + "control"
	app.nodePingInterval = int64(viper.GetInt("node_ping_interval"))
	app.nodeInfoCleanInterval = app.nodePingInterval * 3
	app.nodeInfoMaxDelay = app.nodePingInterval*2 + 1
	app.presencePingInterval = int64(viper.GetInt("presence_ping_interval"))
	app.presenceExpireInterval = int64(viper.GetInt("presence_expire_interval"))
	app.privateChannelPrefix = viper.GetString("private_channel_prefix")
	app.namespaceChannelBoundary = viper.GetString("namespace_channel_boundary")
	app.userChannelBoundary = viper.GetString("user_channel_boundary")
	app.userChannelSeparator = viper.GetString("user_channel_separator")
	app.expiredConnectionCloseDelay = int64(viper.GetInt("expired_connection_close_delay"))
	app.nodeInfoPublishInterval = int64(viper.GetInt("node_info_publish_interval"))
	app.insecure = viper.GetBool("insecure")
	if app.insecure {
		logger.WARN.Println("application initialized in INSECURE MODE")
	}

	app.name = getApplicationName()

	// get and initialize structure
	var pl projectList
	viper.MarshalKey("structure", &pl)
	s := &structure{
		ProjectList: pl,
	}
	s.initialize()
	app.structure = s
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
	case app.controlChannel:
		return app.handleControlMessage(message)
	case app.adminChannel:
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
	return app.engine.publish(app.controlChannel, message)
}

// publishAdminMessage publishes message into admin channel so all running
// nodes will receive it and send to admins connected
func (app *application) publishAdminMessage(message []byte) error {
	return app.engine.publish(app.adminChannel, message)
}

// publishClientMessage publishes message into channel so all running nodes
// will receive it and will send to all clients on node subscribed on channel
func (app *application) publishClientMessage(p *project, channel string, data, info interface{}) error {

	uid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	message := map[string]interface{}{
		"uid":       uid.String(),
		"timestamp": strconv.FormatInt(time.Now().Unix(), 10),
		"info":      info,
		"channel":   channel,
		"data":      data,
	}

	channelOptions := app.getChannelOptions(p.Name, channel)
	if channelOptions == nil {
		return ErrNamespaceNotFound
	}

	if channelOptions.Watch {
		resp := newResponse("message")
		resp.Body = map[string]interface{}{
			"project": p.Name,
			"message": message,
		}
		messageBytes, err := resp.toJson()
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

	byteMessage, err := resp.toJson()
	if err != nil {
		logger.ERROR.Println(err)
		return err
	}

	err = app.engine.publish(projectChannel, byteMessage)
	if err != nil {
		logger.ERROR.Println(err)
		return err
	}

	if channelOptions.HistorySize > 0 {
		err := app.addHistoryMessage(p.Name, channel, message, channelOptions.HistorySize, channelOptions.HistoryLifetime)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	return nil
}

// publishJoinLeaveMessage allows to publish join message into channel when
// someone subscribes on it or leave message when someone unsubscribed from channel
func (app *application) publishJoinLeaveMessage(projectKey, channel, method string, info map[string]interface{}) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	resp := newResponse(method)
	resp.Body = map[string]interface{}{
		"channel": channel,
		"data":    info,
	}
	byteMessage, err := resp.toJson()
	if err != nil {
		return err
	}
	return app.engine.publish(projectChannel, byteMessage)
}

func (app *application) publishPingControlMessage() error {
	message := map[string]interface{}{
		"uid":    app.uid,
		"method": "ping",
		"params": map[string]string{
			"uid":  app.uid,
			"name": app.name,
		},
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
		Uid:       cmd.Uid,
		Name:      cmd.Name,
		UpdatedAt: time.Now().Unix(),
	}
	app.nodes[cmd.Uid] = info
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
	return app.channelPrefix + "." + projectKey + "." + channel
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

// unsubscribeUserFromChannel allows to unsubscribe user...wait for it...from channel! If channel
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
		err := c.close("default")
		if err != nil {
			return err
		}
	}
	return nil
}

// getProjectByKey returns a project by project key (name) using structure
func (app *application) getProjectByKey(projectKey string) (*project, bool) {
	return app.structure.getProjectByKey(projectKey)
}

// extractNamespaceName returns namespace name from channel name if exists or
// empty string
func (app *application) extractNamespaceName(channel string) string {
	channel = strings.TrimPrefix(channel, app.privateChannelPrefix)
	parts := strings.SplitN(channel, app.namespaceChannelBoundary, 2)
	if len(parts) >= 2 {
		return parts[0]
	} else {
		return ""
	}
}

// getChannelOptions returns channel options for channel using structure
func (app *application) getChannelOptions(projectKey, channel string) *ChannelOptions {
	namespaceName := app.extractNamespaceName(channel)
	return app.structure.getChannelOptions(projectKey, namespaceName)
}

// addPresence proxies presence adding to engine
func (app *application) addPresence(projectKey, channel, uid string, info interface{}) error {
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
func (app *application) addHistoryMessage(projectKey, channel string, message interface{}, size, lifetime int64) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.addHistoryMessage(projectChannel, message, size, lifetime)
}

// getHistory proxies history extraction to engine
func (app *application) getHistory(projectKey, channel string) ([]interface{}, error) {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.getHistory(projectChannel)
}

// isPrivateChannel checks if channel private and therefore subscription
// request on it must be properly signed on web application backend
func (app *application) isPrivateChannel(channel string) bool {
	return strings.HasPrefix(channel, app.privateChannelPrefix)
}

// isUserAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it
func (app *application) isUserAllowed(channel, user string) bool {
	if !strings.Contains(channel, app.userChannelBoundary) {
		return true
	}
	parts := strings.Split(channel, app.userChannelBoundary)
	allowedUsers := strings.Split(parts[len(parts)-1], app.userChannelSeparator)
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
