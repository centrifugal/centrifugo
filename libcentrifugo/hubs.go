package libcentrifugo

import (
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// clientHub manages client connections
type clientHub struct {
	sync.RWMutex

	// registry to hold active connections
	// as map[of projects]map[of user IDs]map[unique connection IDs]connection
	connections map[ProjectKey]map[UserID]map[ConnID]clientConn
}

// newClientHub initializes connectionHub
func newClientHub() *clientHub {
	return &clientHub{
		connections: make(map[ProjectKey]map[UserID]map[ConnID]clientConn),
	}
}

// nClients returns total number of client connections
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

// nUniqueClients returns a number of unique users connected
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
		h.connections[project] = make(map[UserID]map[ConnID]clientConn)
	}
	_, ok = h.connections[project][user]
	if !ok {
		h.connections[project][user] = make(map[ConnID]clientConn)
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

// userConnections returns all connections of user with UserID in project
func (h *clientHub) userConnections(pk ProjectKey, user UserID) map[ConnID]clientConn {
	h.RLock()
	defer h.RUnlock()

	_, ok := h.connections[pk]
	if !ok {
		return map[ConnID]clientConn{}
	}

	userConnections, ok := h.connections[pk][user]
	if !ok {
		return map[ConnID]clientConn{}
	}

	var conns map[ConnID]clientConn
	conns = make(map[ConnID]clientConn, len(userConnections))
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
	subs map[ChannelID]map[ConnID]clientConn
}

// newSubHub initializes subscriptionHub
func newSubHub() *subHub {
	return &subHub{
		subs: make(map[ChannelID]map[ConnID]clientConn),
	}
}

// nChannels returns a total number of different channels
func (h *subHub) nChannels() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.subs)
}

// channels returns a slice of all active channels
func (h *subHub) channels() []ChannelID {
	h.RLock()
	defer h.RUnlock()
	channels := make([]ChannelID, len(h.subs))
	i := 0
	for ch := range h.subs {
		channels[i] = ch
		i += 1
	}
	return channels
}

// add adds connection into clientSubscriptionHub subscriptions registry
func (h *subHub) add(chID ChannelID, c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	_, ok := h.subs[chID]
	if !ok {
		h.subs[chID] = make(map[ConnID]clientConn)
	}
	h.subs[chID][uid] = c
	return nil
}

// remove removes connection from clientSubscriptionHub subscriptions registry
func (h *subHub) remove(chID ChannelID, c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	// try to find subscription to delete, return early if not found
	if _, ok := h.subs[chID]; !ok {
		return nil
	}
	if _, ok := h.subs[chID][uid]; !ok {
		return nil
	}

	// actually remove subscription from hub
	delete(h.subs[chID], uid)

	// clean up map if it's needed
	if len(h.subs[chID]) == 0 {
		delete(h.subs, chID)
	}

	return nil
}

// broadcast sends message to all clients subscribed on channel
func (h *subHub) broadcast(chID ChannelID, message string) error {
	h.RLock()
	defer h.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subs[chID]
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

// adminHub manages admin connections from web interface
type adminHub struct {
	sync.RWMutex

	// registry to hold active admin connections
	// as map[unique admin connection IDs]*connection
	connections map[ConnID]adminConn
}

// newAdminHub initializes new adminHub
func newAdminHub() *adminHub {
	return &adminHub{
		connections: make(map[ConnID]adminConn),
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
