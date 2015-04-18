package main

// TODO: use interfaces instead of app reference in client and engine

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
)

type application struct {
	sync.Mutex

	// unique id for this application (node)
	uid string
	// name of this node - based on hostname and port
	name string
	// nodes is a map with information about nodes known
	nodes map[string]interface{}

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

	// prefix before each channel
	channelPrefix string
	// channel name for admin messages
	adminChannel string
	// channel name for internal control messages between nodes
	controlChannel string

	// in seconds, how often connected clients must update presence info
	presencePingInterval int
	// in seconds, how long to consider presence info valid after receiving presence ping
	presenceExpireInterval int

	// prefix in channel name which indicates that channel is private
	privateChannelPrefix string
	// string separator which must be put after namespace part in channel name
	namespaceChannelBoundary string
	// string separator which must be set before allowed users part in channel name
	userChannelBoundary string
	// separates allowed users in user part of channel name
	userChannelSeparator string
}

func newApplication() (*application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &application{
		uid:                   uid.String(),
		nodes:                 make(map[string]interface{}),
		clientConnectionHub:   newClientConnectionHub(),
		clientSubscriptionHub: newClientSubscriptionHub(),
		adminConnectionHub:    newAdminConnectionHub(),
	}, nil
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
		log.Println(err)
		hostname = "?"
	}
	return hostname + "_" + port
}

// initialize used to set configuration dependent application properties
func (app *application) initialize() {
	app.Lock()
	defer app.Unlock()
	app.channelPrefix = viper.GetString("channel_prefix")
	app.adminChannel = app.channelPrefix + "." + "admin"
	app.controlChannel = app.channelPrefix + "." + "control"
	app.presencePingInterval = viper.GetInt("presence_ping_interval")
	app.presenceExpireInterval = viper.GetInt("presence_expire_interval")
	app.privateChannelPrefix = viper.GetString("private_channel_prefix")
	app.namespaceChannelBoundary = viper.GetString("namespace_channel_boundary")
	app.userChannelBoundary = viper.GetString("user_channel_boundary")
	app.userChannelSeparator = viper.GetString("user_channel_separator")
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
func (app *application) handleMessage(channel, message string) error {
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
func (app *application) handleControlMessage(message string) error {

	// TODO: implement this
	var cmd controlCommand
	err := json.Unmarshal([]byte(message), &cmd)
	if err != nil {
		log.Println(err)
		return err
	}

	if cmd.Uid == app.uid {
		// sent by this node
		return nil
	}

	method := cmd.Method
	switch method {
	case "ping":
		err = app.handlePingControlMessage(cmd.Params)
	case "unsubscribe":
		err = app.handleUnsubscribeControlMessage(cmd.Params)
	case "disconnect":
		err = app.handleDisconnectControlMessage(cmd.Params)
	default:
		log.Println("unknown control message method", method)
	}

	return nil
}

// handleAdminMessage handles messages from admin channel - those messages
// must be delivered to all admins connected to this node
func (app *application) handleAdminMessage(message string) error {
	return app.adminConnectionHub.broadcast(message)
}

// handleClientMessage handles messages published by web application or client
// into channel. The goal of this method to deliver this message to all clients
// on this node subscribed on channel
func (app *application) handleClientMessage(channel, message string) error {
	return app.clientSubscriptionHub.broadcast(channel, message)
}

// publishControlMessage publishes message into control channel so all running
// nodes will receive it and will process it
func (app *application) publishControlMessage(messageData map[string]interface{}) error {
	message, err := json.Marshal(messageData)
	if err != nil {
		log.Println(err)
		return err
	}
	return app.engine.publish(app.controlChannel, string(message))
}

// publishAdminMessage publishes message into admin channel so all running
// nodes will receive it and send to admins connected
func (app *application) publishAdminMessage(message string) error {
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
	if channelOptions.Watch {
		resp := newResponse("message")
		resp.Body = map[string]interface{}{
			"project": p.Name,
			"message": message,
		}
		messageBytes, err := resp.toJson()
		if err != nil {
			log.Println(err)
		} else {
			err = app.publishAdminMessage(string(messageBytes))
			if err != nil {
				log.Println(err)
			}
		}
	}

	projectChannel := app.getProjectChannel(p.Name, channel)

	resp := newResponse("message")
	resp.Body = message

	byteMessage, err := resp.toJson()
	if err != nil {
		log.Println(err)
		return err
	}

	err = app.engine.publish(projectChannel, string(byteMessage))
	if err != nil {
		log.Println(err)
		return err
	}

	if channelOptions.HistorySize > 0 {
		// TODO: add message to history
		log.Println("adding message in history must be implemented here")
	}

	return nil
}

func (app *application) handlePingControlMessage(params Params) error {
	return nil
}

func (app *application) handleUnsubscribeControlMessage(params Params) error {
	return nil
}

func (app *application) handleDisconnectControlMessage(params Params) error {
	return nil
}

// getProjectChannel returns internal name of channel - as
// every project can have channels with the same name we should distinguish
// between them. This also prevents collapses with admin and control
// channel names
func (app *application) getProjectChannel(projectKey, channel string) string {
	return app.channelPrefix + "." + projectKey + "." + channel
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

func (app *application) unsubscribeUserFromChannel(user, channel string) error {

	// TODO: implement this

	return nil
}

// getProjectByKey returns a project by project key (name) using structure
func (app *application) getProjectByKey(projectKey string) (*project, bool) {
	return app.structure.getProjectByKey(projectKey)
}

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
func (app *application) addHistoryMessage(projectKey, channel string, message interface{}) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.addHistoryMessage(projectChannel, message)
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
