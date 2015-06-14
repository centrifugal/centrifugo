package libcentrifugo

import (
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// clientHub manages client connections
type clientHub struct {
	sync.RWMutex

	conns map[ConnID]clientConn

	// registry to hold active connections grouped by project and user ID
	connections map[ProjectKey]map[UserID]map[ConnID]bool

	// registry to hold active subscriptions of clients on channels
	subs map[ChannelID]map[ConnID]bool
}

// newClientHub initializes connectionHub
func newClientHub() *clientHub {
	return &clientHub{
		conns:       make(map[ConnID]clientConn),
		connections: make(map[ProjectKey]map[UserID]map[ConnID]bool),
		subs:        make(map[ChannelID]map[ConnID]bool),
	}
}

// shutdown unsubscribes users from all channels and disconnects them
func (h *clientHub) shutdown() {
	var wg sync.WaitGroup
	h.RLock()
	for _, uc := range h.connections {
		for _, user := range uc {
			wg.Add(len(user))
			for uid, _ := range user {
				cc, ok := h.conns[uid]
				if !ok {
					continue
				}
				go func(cc clientConn) {
					for _, ch := range cc.channels() {
						cc.unsubscribe(ch)
					}
					cc.close("shutting down")
					wg.Done()
				}(cc)
			}
		}
	}
	h.RUnlock()
	wg.Wait()
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

	h.conns[uid] = c

	_, ok := h.connections[project]
	if !ok {
		h.connections[project] = make(map[UserID]map[ConnID]bool)
	}
	_, ok = h.connections[project][user]
	if !ok {
		h.connections[project][user] = make(map[ConnID]bool)
	}
	h.connections[project][user][uid] = true
	return nil
}

// remove removes connection from clientConnectionHub connections registry
func (h *clientHub) remove(c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()
	user := c.user()
	project := c.project()

	delete(h.conns, uid)

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
	for uid, _ := range userConnections {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
		conns[uid] = c
	}

	return conns
}

// nChannels returns a total number of different channels
func (h *clientHub) nChannels() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.subs)
}

// channels returns a slice of all active channels
func (h *clientHub) channels() []ChannelID {
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
func (h *clientHub) addSub(chID ChannelID, c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	h.conns[uid] = c

	_, ok := h.subs[chID]
	if !ok {
		h.subs[chID] = make(map[ConnID]bool)
	}
	h.subs[chID][uid] = true
	return nil
}

// remove removes connection from clientSubscriptionHub subscriptions registry
func (h *clientHub) removeSub(chID ChannelID, c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	delete(h.conns, uid)

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
func (h *clientHub) broadcast(chID ChannelID, message string) error {
	h.RLock()
	defer h.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subs[chID]
	if !ok {
		return nil
	}

	// iterate over them and send message individually
	for uid, _ := range channelSubscriptions {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
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
