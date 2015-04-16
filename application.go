package main

import (
	"log"
	"os"
	"sync"

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

func (app *application) processPublish(p *project, channel string, data, info interface{}) (bool, error) {

	// TODO: implement this

	return true, nil
}

// getEngineChannel returns a name of channel used by engine - as
// every project can have channels with the same name we should distinguish
// between them. This also prevents collapses with admin and control
// channel names
func (app *application) getEngineChannel(projectKey, channel string) string {
	return app.channelPrefix + "." + projectKey + "." + channel
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
	engineChannel := app.getEngineChannel(projectKey, channel)
	return app.engine.getPresence(engineChannel)
}

// getHistory proxies history extraction to engine
func (app *application) getHistory(projectKey, channel string) (interface{}, error) {
	engineChannel := app.getEngineChannel(projectKey, channel)
	return app.engine.getHistory(engineChannel)
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
