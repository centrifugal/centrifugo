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
	hub *hub
	// adminHub to manage admin connections
	adminHub *adminHub
	// nodes is a map with information about nodes known
	nodes map[string]interface{}
	// engine to use - in memory or redis
	engine string
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

func newApplication(engine string) (*application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &application{
		uid:      uid.String(),
		nodes:    make(map[string]interface{}),
		engine:   engine,
		hub:      newHub(),
		adminHub: newAdminHub(),
		name:     getApplicationName(),
	}, nil
}

// initialize used to set configuration dependent application properties
func (app *application) initialize() {
	app.channelPrefix = viper.GetString("channel_prefix")
	app.adminChannel = app.channelPrefix + "." + "admin"
	app.controlChannel = app.channelPrefix + "." + "control"
	app.presencePingInterval = viper.GetInt("presence_ping_interval")
	app.presenceExpireInterval = viper.GetInt("presence_expire_interval")
}

func (app *application) setStructure(s *structure) {
	app.Lock()
	defer app.Unlock()
	app.structure = s
}

func (app *application) processPublish(p *project, channel string, data, info interface{}) (bool, error) {

	// TODO: implement this

	return true, nil
}

func (app *application) processPresence(p *project, channel string) (interface{}, error) {

	// TODO: implement this

	return map[string]interface{}{}, nil
}

func (app *application) processHistory(p *project, channel string) (interface{}, error) {

	// TODO: implement this

	return map[string]interface{}{}, nil
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
