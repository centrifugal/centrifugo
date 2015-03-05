package main

import (
	"time"

	"github.com/nu7hatch/gouuid"
)

type application struct {
	uid          string
	hub          interface{}
	adminHub     interface{}
	nodes        map[string]interface{}
	engine       string
	revisionTime time.Time
	address      string
}

func newApplication(engine string) (*application, error) {
	uid, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	return &application{
		uid:    uid,
		nodes:  make(map[string]interface{}),
		engine: engine,
	}, nil
}

func (app *application) addConnection(cl *client) error {
	return app.hub.add(cl)
}

func (app *application) removeConnection(cl *client) error {
	return app.hub.remove(cl)
}

func (app *application) addAdminConnection(cl *adminClient) error {
	return app.adminHub.add(cl)
}

func (app *application) removeAdminConnection(cl *adminClient) error {
	return app.adminHub.remove(cl)
}
