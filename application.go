package main

import (
	"time"

	"github.com/nu7hatch/gouuid"
)

type application struct {
	uid          string
	hub          *hub
	adminHub     *adminHub
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
		uid:      uid.String(),
		nodes:    make(map[string]interface{}),
		engine:   engine,
		hub:      newHub(),
		adminHub: newAdminHub(),
	}, nil
}
