package main

import (
	"sync"
)

type memoryEngine struct {
	app         *application
	presenceHub *memoryPresenceHub
	historyHub  *memoryHistoryHub
}

func newMemoryEngine(app *application) *memoryEngine {
	return &memoryEngine{
		app:         app,
		presenceHub: newMemoryPresenceHub(),
		historyHub:  newMemoryHistoryHub(),
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
	return e.presenceHub.add(channel, uid, info)
}

func (e *memoryEngine) removePresence(channel, uid string) error {
	return e.presenceHub.remove(channel, uid)
}

func (e *memoryEngine) getPresence(channel string) (map[string]interface{}, error) {
	return e.presenceHub.get(channel)
}

func (e *memoryEngine) addHistoryMessage(channel string, message interface{}) error {
	return e.historyHub.add(channel, message)
}

func (e *memoryEngine) getHistory(channel string) ([]interface{}, error) {
	return e.historyHub.get(channel)
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

func (h *memoryPresenceHub) add(channel, uid string, info interface{}) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[channel]
	if !ok {
		h.presence[channel] = make(map[string]interface{})
	}
	h.presence[channel][uid] = info
	return nil
}

func (h *memoryPresenceHub) remove(channel, uid string) error {
	h.Lock()
	defer h.Unlock()

	if _, ok := h.presence[channel]; !ok {
		return nil
	}
	if _, ok := h.presence[channel][uid]; !ok {
		return nil
	}

	delete(h.presence[channel], uid)

	// clean up map if needed
	if len(h.presence[channel]) == 0 {
		delete(h.presence, channel)
	}

	return nil
}

func (h *memoryPresenceHub) get(channel string) (map[string]interface{}, error) {
	h.Lock()
	defer h.Unlock()

	presence, ok := h.presence[channel]
	if !ok {
		// return empty map
		return map[string]interface{}{}, nil
	}
	return presence, nil
}

type memoryHistoryHub struct {
	sync.Mutex
	history map[string][]interface{}
}

func newMemoryHistoryHub() *memoryHistoryHub {
	return &memoryHistoryHub{
		history: make(map[string][]interface{}),
	}
}

func (h *memoryHistoryHub) add(channel string, message interface{}) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.history[channel]
	if !ok {
		h.history[channel] = []interface{}{}
	}
	h.history[channel] = append(h.history[channel], message)
	return nil
}

func (h *memoryHistoryHub) get(channel string) ([]interface{}, error) {
	h.Lock()
	defer h.Unlock()

	history, ok := h.history[channel]
	if !ok {
		// return empty slice
		return []interface{}{}, nil
	}
	return history, nil
}
