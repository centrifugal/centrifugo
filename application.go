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

func (app *application) addConnection(c *connection) error {
	return app.hub.add(c)
}

func (app *application) removeConnection(c *connection) error {
	return app.hub.remove(c)
}

func (app *application) addAdminConnection(c *connection) error {
	return app.adminHub.add(c)
}

func (app *application) removeAdminConnection(c *connection) error {
	return app.adminHub.remove(c)
}
