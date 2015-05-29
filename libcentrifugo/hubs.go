package libcentrifugo

import (
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// clientConnectionHub manages client connections
type clientHub struct {
	sync.RWMutex

	// registry to hold active connections
	// as map[of projects]map[of user IDs]map[unique connection IDs]connection
	connections map[ProjectKey]map[UserID]map[string]clientConn
}

// newClientHub initializes connectionHub
func newClientHub() *clientHub {
	return &clientHub{
		connections: make(map[ProjectKey]map[UserID]map[string]clientConn),
	}
}

func (h *clientHub) nClients() int {
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

func (h *clientHub) nUniqueClients() int {
	h.RLock()
	defer h.RUnlock()
	total := 0
	for _, userConnections := range h.connections {
		total += len(userConnections)
	}
	return total
}

// add adds connection into clientConnectionHub connections registry
func (h *clientHub) add(c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()
	user := c.user()
	project := c.project()

	_, ok := h.connections[project]
	if !ok {
		h.connections[project] = make(map[UserID]map[string]clientConn)
	}
	_, ok = h.connections[project][user]
	if !ok {
		h.connections[project][user] = make(map[string]clientConn)
	}
	h.connections[project][user][uid] = c
	return nil
}

// remove removes connection from clientConnectionHub connections registry
func (h *clientHub) remove(c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()
	user := c.user()
	project := c.project()

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

func (h *clientHub) userConnections(projectKey ProjectKey, user UserID) map[string]clientConn {
	h.RLock()
	defer h.RUnlock()

	_, ok := h.connections[projectKey]
	if !ok {
		return map[string]clientConn{}
	}

	userConnections, ok := h.connections[projectKey][user]
	if !ok {
		return map[string]clientConn{}
	}

	var conns map[string]clientConn
	conns = make(map[string]clientConn, len(userConnections))
	for k, v := range userConnections {
		conns[k] = v
	}

	return conns
}

// subHub manages client subscriptions on channels
type subHub struct {
	sync.RWMutex

	// registry to hold active subscriptions of clients on channels
	// as map[of engine channel]map[of connection UID]*connection
	subs map[string]map[string]clientConn
}

// newSubHub initializes subscriptionHub
func newSubHub() *subHub {
	return &subHub{
		subs: make(map[string]map[string]clientConn),
	}
}

func (h *subHub) nChannels() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.subs)
}

func (h *subHub) channels() []string {
	h.RLock()
	defer h.RUnlock()
	channels := make([]string, len(h.subs))
	i := 0
	for ch := range h.subs {
		channels[i] = ch
		i += 1
	}
	return channels
}

// add adds connection into clientSubscriptionHub subscriptions registry
func (h *subHub) add(channel string, c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	_, ok := h.subs[channel]
	if !ok {
		h.subs[channel] = make(map[string]clientConn)
	}
	h.subs[channel][uid] = c
	return nil
}

// remove removes connection from clientSubscriptionHub subscriptions registry
func (h *subHub) remove(channel string, c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	// try to find subscription to delete, return early if not found
	if _, ok := h.subs[channel]; !ok {
		return nil
	}
	if _, ok := h.subs[channel][uid]; !ok {
		return nil
	}

	// actually remove subscription from hub
	delete(h.subs[channel], uid)

	// clean up map if it's needed
	if len(h.subs[channel]) == 0 {
		delete(h.subs, channel)
	}

	return nil
}

// broadcast sends message to all clients subscribed on channel
func (h *subHub) broadcast(channel, message string) error {
	h.RLock()
	defer h.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subs[channel]
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
type adminHub struct {
	sync.RWMutex

	// registry to hold active admin connections
	// as map[unique admin connection IDs]*connection
	connections map[string]adminConn
}

// newAdminHub initializes new adminHub
func newAdminHub() *adminHub {
	return &adminHub{
		connections: make(map[string]adminConn),
	}
}

// add adds connection to adminConnectionHub connections registry
func (h *adminHub) add(c adminConn) error {
	h.Lock()
	defer h.Unlock()
	h.connections[c.uid()] = c
	return nil
}

// remove removes connection from adminConnectionHub connections registry
func (h *adminHub) remove(c adminConn) error {
	h.Lock()
	defer h.Unlock()
	delete(h.connections, c.uid())
	return nil
}

// broadcast sends message to all connected admins
func (h *adminHub) broadcast(message string) error {
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
