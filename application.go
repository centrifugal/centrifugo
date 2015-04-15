package main

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/nu7hatch/gouuid"
	"github.com/spf13/viper"
)

type application struct {
	sync.Mutex
	uid          string
	hub          *hub
	adminHub     *adminHub
	nodes        map[string]interface{}
	engine       string
	revisionTime time.Time
	name         string
	structure    *structure
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
