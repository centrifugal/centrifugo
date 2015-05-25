package libcentrifugo

import (
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// clientConnectionHub manages client connections
type clientConnectionHub struct {
	sync.RWMutex

	// registry to hold active connections
	// as map[of projects]map[of user IDs]map[unique connection IDs]connection
	connections map[string]map[string]map[string]clientConnection
}

// newClientConnectionHub initializes connectionHub
func newClientConnectionHub() *clientConnectionHub {
	return &clientConnectionHub{
		connections: make(map[string]map[string]map[string]clientConnection),
	}
}

func (h *clientConnectionHub) getClientsCount() int {
	h.RLock()
	defer h.RUnlock()
	total := 0
	for _, userConnections := range h.connections {
		for _, clientConnections := range userConnections {
			total += len(clientConnections)
		}
	}
	return total
}

func (h *clientConnectionHub) getUniqueClientsCount() int {
	h.RLock()
	defer h.RUnlock()
	total := 0
	for _, userConnections := range h.connections {
		total += len(userConnections)
	}
	return total
}

// add adds connection into clientConnectionHub connections registry
func (h *clientConnectionHub) add(c clientConnection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.getUid()
	user := c.getUser()
	project := c.getProject()

	_, ok := h.connections[project]
	if !ok {
		h.connections[project] = make(map[string]map[string]clientConnection)
	}
	_, ok = h.connections[project][user]
	if !ok {
		h.connections[project][user] = make(map[string]clientConnection)
	}
	h.connections[project][user][uid] = c
	return nil
}

// remove removes connection from clientConnectionHub connections registry
func (h *clientConnectionHub) remove(c clientConnection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.getUid()
	user := c.getUser()
	project := c.getProject()

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

func (h *clientConnectionHub) getUserConnections(projectKey, user string) map[string]clientConnection {
	h.RLock()
	defer h.RUnlock()

	_, ok := h.connections[projectKey]
	if !ok {
		return map[string]clientConnection{}
	}

	userConnections, ok := h.connections[projectKey][user]
	if !ok {
		return map[string]clientConnection{}
	}
	return userConnections
}

// clientSubscriptionHub manages client subscriptions on channels
type clientSubscriptionHub struct {
	sync.RWMutex

	// registry to hold active subscriptions of clients on channels
	// as map[of engine channel]map[of connection UID]*connection
	subscriptions map[string]map[string]clientConnection
}

// newClientSubscriptionHub initializes subscriptionHub
func newClientSubscriptionHub() *clientSubscriptionHub {
	return &clientSubscriptionHub{
		subscriptions: make(map[string]map[string]clientConnection),
	}
}

func (h *clientSubscriptionHub) getChannelsCount() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.subscriptions)
}

func (h *clientSubscriptionHub) getChannels() []string {
	h.RLock()
	defer h.RUnlock()
	channels := make([]string, len(h.subscriptions))
	i := 0
	for ch := range h.subscriptions {
		channels[i] = ch
		i += 1
	}
	return channels
}

// add adds connection into clientSubscriptionHub subscriptions registry
func (h *clientSubscriptionHub) add(channel string, c clientConnection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.getUid()

	_, ok := h.subscriptions[channel]
	if !ok {
		h.subscriptions[channel] = make(map[string]clientConnection)
	}
	h.subscriptions[channel][uid] = c
	return nil
}

// remove removes connection from clientSubscriptionHub subscriptions registry
func (h *clientSubscriptionHub) remove(channel string, c clientConnection) error {
	h.Lock()
	defer h.Unlock()

	uid := c.getUid()

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

// broadcast sends message to all clients subscribed on channel
func (h *clientSubscriptionHub) broadcast(channel, message string) error {
	h.RLock()
	defer h.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subscriptions[channel]
	if !ok {
		return nil
	}

	// iterate over them and send message individually
	for _, c := range channelSubscriptions {
		err := c.send(message)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}
	return nil
}

// adminConnectionHub manages admin connections from web interface
type adminConnectionHub struct {
	sync.RWMutex

	// registry to hold active admin connections
	// as map[unique admin connection IDs]*connection
	connections map[string]adminConnection
}

// newAdminConnectionHub initializes new adminHub
func newAdminConnectionHub() *adminConnectionHub {
	return &adminConnectionHub{
		connections: make(map[string]adminConnection),
	}
}

// add adds connection to adminConnectionHub connections registry
func (h *adminConnectionHub) add(c adminConnection) error {
	h.Lock()
	defer h.Unlock()
	h.connections[c.getUid()] = c
	return nil
}

// remove removes connection from adminConnectionHub connections registry
func (h *adminConnectionHub) remove(c adminConnection) error {
	h.Lock()
	defer h.Unlock()
	delete(h.connections, c.getUid())
	return nil
}

// broadcast sends message to all connected admins
func (h *adminConnectionHub) broadcast(message string) error {
	h.RLock()
	defer h.RUnlock()
	for _, c := range h.connections {
		err := c.send(message)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}
	return nil
}
