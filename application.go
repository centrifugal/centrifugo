package main

// TODO: use interfaces instead of app reference in client and engine

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
)

type application struct {
	sync.Mutex

	// unique id for this application (node)
	uid string
	// hub to manage client connections
	connectionHub *connectionHub
	// hub to manage client subscriptions
	subscriptionHub *subscriptionHub
	// hub to manage admin connections
	adminConnectionHub *adminConnectionHub
	// nodes is a map with information about nodes known
	nodes map[string]interface{}
	// engine to use - in memory or redis
	engine engine
	// name of this node - based on hostname and port
	name string
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
}

func newApplication() (*application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &application{
		uid:                uid.String(),
		nodes:              make(map[string]interface{}),
		connectionHub:      newConnectionHub(),
		subscriptionHub:    newSubscriptionHub(),
		adminConnectionHub: newAdminConnectionHub(),
	}, nil
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

func (app *application) setEngine(e engine) {
	app.Lock()
	defer app.Unlock()
	app.engine = e
}

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

func (app *application) handleControlMessage(message string) error {

	// TODO: implement this

	return nil
}

func (app *application) handleAdminMessage(message string) error {
	return app.adminConnectionHub.broadcast(message)
}

func (app *application) handleClientMessage(channel, message string) error {
	return app.subscriptionHub.broadcast(channel, message)
}

func (app *application) publishClientMessage(p *project, channel string, data, clientInfo interface{}) error {

	uid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	byteMessage, err := json.Marshal(map[string]interface{}{
		"uid":       uid.String(),
		"timestamp": strconv.FormatInt(time.Now().Unix(), 10),
		"client":    clientInfo,
		"channel":   channel,
		"data":      data,
	})
	if err != nil {
		log.Println(err)
		return err
	}

	message := string(byteMessage)

	channelOptions := app.getChannelOptions(p.Name, channel)
	if channelOptions.Watch {
		// TODO: send admin message
		log.Println("publish admin message must be implemented here")
	}

	projectChannel := app.getProjectChannel(p.Name, channel)
	err = app.engine.publish(projectChannel, message)
	if err != nil {
		log.Println(err)
		return err
	}

	if channelOptions.History {
		// TODO: add message to history
		log.Println("adding message in history must be implemented here")
	}

	return nil
}

// getProjectChannel returns internal name of channel - as
// every project can have channels with the same name we should distinguish
// between them. This also prevents collapses with admin and control
// channel names
func (app *application) getProjectChannel(projectKey, channel string) string {
	return app.channelPrefix + "." + projectKey + "." + channel
}

func (app *application) addSubscription(projectKey, channel string, c connection) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	err := app.engine.subscribe(projectChannel)
	if err != nil {
		return err
	}
	return app.subscriptionHub.add(projectChannel, c)
}

func (app *application) removeSubscription(projectKey, channel string, c connection) error {
	projectChannel := app.getProjectChannel(projectKey, channel)
	err := app.engine.unsubscribe(projectChannel)
	if err != nil {
		return err
	}
	return app.subscriptionHub.remove(projectChannel, c)
}

func (app *application) getSubscriptions(projectKey, channel string) map[string]connection {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.subscriptionHub.get(projectChannel)
}

// getProjectByKey returns a project by project key (name) using structure
func (app *application) getProjectByKey(projectKey string) (*project, bool) {
	return app.structure.getProjectByKey(projectKey)
}

// getChannelOptions returns channel options for channel using structure
func (app *application) getChannelOptions(projectKey, channel string) *ChannelOptions {
	return app.structure.getChannelOptions(projectKey, channel)
}

// getPresence proxies presence extraction to engine
func (app *application) getPresence(projectKey, channel string) (interface{}, error) {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.getPresence(projectChannel)
}

// getHistory proxies history extraction to engine
func (app *application) getHistory(projectKey, channel string) (interface{}, error) {
	projectChannel := app.getProjectChannel(projectKey, channel)
	return app.engine.getHistory(projectChannel)
}

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

func isPrivateChannel(channel string) bool {

	// TODO: implement this

	return false
}
