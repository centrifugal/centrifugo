package main

import (
	"log"
	"sync"
)

// connectionHub manages client connections
type connectionHub struct {
	sync.Mutex

	// registry to hold active connections
	// as map[of projects]map[of user IDs]map[unique connection IDs]connection
	connections map[string]map[string]map[string]connection
}

// newConnectionHub initializes connectionHub
func newConnectionHub() *connectionHub {
	return &connectionHub{
		connections: make(map[string]map[string]map[string]connection),
	}
}

// add adds connection into connectionHub connections registry
func (h *connectionHub) add(c connection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.GetUid()
	user := c.GetUser()
	project := c.GetProject()

	_, ok := h.connections[project]
	if !ok {
		h.connections[project] = make(map[string]map[string]connection)
	}
	_, ok = h.connections[project][user]
	if !ok {
		h.connections[project][user] = make(map[string]connection)
	}
	h.connections[project][user][uid] = c
	return nil
}

// remove removes connection from connectionHub connections registry
func (h *connectionHub) remove(c connection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.GetUid()
	user := c.GetUser()
	project := c.GetProject()

	// try to find connection to delete, return early if not found
	if _, ok := h.connections[project]; !ok {
		return nil
	}
	if _, ok := h.connections[project][user]; !ok {
		return nil
	}
	if _, ok := h.connections[project][user][uid]; !ok {
		return nil
	}

	// actually remove connection from hub
	delete(h.connections[project][user], uid)

	// clean up map if it's needed
	if len(h.connections[project][user]) == 0 {
		delete(h.connections[project], user)
	}
	if len(h.connections[project]) == 0 {
		delete(h.connections, project)
	}

	return nil
}

// subscriptionHub manages client subscriptions on channels
type subscriptionHub struct {
	sync.Mutex

	// registry to hold active subscriptions of clients on channels
	// as map[of engine channel]map[of connection UID]*connection
	subscriptions map[string]map[string]connection
}

// newSubscriptionHub initializes subscriptionHub
func newSubscriptionHub() *subscriptionHub {
	return &subscriptionHub{
		subscriptions: make(map[string]map[string]connection),
	}
}

// add adds connection into subscriptionHub subscriptions registry
func (h *subscriptionHub) add(channel string, c connection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.GetUid()

	_, ok := h.subscriptions[channel]
	if !ok {
		h.subscriptions[channel] = make(map[string]connection)
	}
	h.subscriptions[channel][uid] = c
	return nil
}

// remove removes connection from connectionHub connections registry
func (h *subscriptionHub) remove(channel string, c connection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.GetUid()

	// try to find subscription to delete, return early if not found
	if _, ok := h.subscriptions[channel]; !ok {
		return nil
	}
	if _, ok := h.subscriptions[channel][uid]; !ok {
		return nil
	}

	// actually remove subscription from hub
	delete(h.subscriptions[channel], uid)

	// clean up map if it's needed
	if len(h.subscriptions[channel]) == 0 {
		delete(h.subscriptions, channel)
	}

	return nil
}

func (h *subscriptionHub) broadcast(channel, message string) error {
	h.Lock()
	defer h.Unlock()
	channelSubscriptions, ok := h.subscriptions[channel]
	if !ok {
		return nil
	}
	log.Println(9)
	for _, c := range channelSubscriptions {
		err := c.Send(message)
		if err != nil {
			log.Println(err)
		}
	}
	log.Println(10)
	return nil
}

// adminConnectionHub manages admin connections from web interface
type adminConnectionHub struct {
	sync.Mutex

	// registry to hold active admin connections
	// as map[unique admin connection IDs]*connection
	connections map[string]connection
}

// newAdminConnectionHub initializes new adminHub
func newAdminConnectionHub() *adminConnectionHub {
	return &adminConnectionHub{
		connections: make(map[string]connection),
	}
}

// add adds connection to adminConnectionHub connections registry
func (h *adminConnectionHub) add(c connection) error {
	h.Lock()
	defer h.Unlock()
	h.connections[c.GetUid()] = c
	return nil
}

// remove removes connection from adminConnectionHub connections registry
func (h *adminConnectionHub) remove(c connection) error {
	h.Lock()
	defer h.Unlock()
	delete(h.connections, c.GetUid())
	return nil
}

func (h *adminConnectionHub) broadcast(message string) error {
	h.Lock()
	defer h.Unlock()
	for _, c := range h.connections {
		err := c.Send(message)
		if err != nil {
			log.Println(err)
		}
	}
	return nil
}
