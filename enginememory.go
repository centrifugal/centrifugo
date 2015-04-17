package main

import (
	"sync"
)

type memoryEngine struct {
	app *application
}

func newMemoryEngine(app *application) *memoryEngine {
	return &memoryEngine{
		app: app,
	}
}

func (e *memoryEngine) getName() string {
	return "In memory – single node only"
}

func (e *memoryEngine) publish(channel, message string) error {
	return e.app.handleMessage(channel, message)
}

func (e *memoryEngine) subscribe(channel string) error {
	return nil
}

func (e *memoryEngine) unsubscribe(channel string) error {
	return nil
}

func (e *memoryEngine) addPresence(channel, uid string, info interface{}) error {
	return nil
}

func (e *memoryEngine) removePresence(channel, uid string) error {
	return nil
}

func (e *memoryEngine) getPresence(channel string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

func (e *memoryEngine) addHistoryMessage(channel string, message string) error {
	return nil
}

func (e *memoryEngine) getHistory(channel string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

type memoryPresenceHub struct {
	sync.Mutex

	presence map[string]map[string]interface{}
}

func newMemoryPresenceHub() *memoryPresenceHub {
	return &memoryPresenceHub{
		presence: make(map[string]map[string]interface{}),
	}
}
