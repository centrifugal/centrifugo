package libcentrifugo

import (
	"sync"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/message"
)

// clientHub manages client connections.
type clientHub struct {
	sync.RWMutex

	// match ConnID with actual client connection.
	conns map[message.ConnID]clientConn

	// registry to hold active client connections grouped by UserID.
	users map[message.UserID]map[message.ConnID]struct{}

	// registry to hold active subscriptions of clients on channels.
	subs map[message.Channel]map[message.ConnID]struct{}
}

// newClientHub initializes clientHub.
func newClientHub() *clientHub {
	return &clientHub{
		conns: make(map[message.ConnID]clientConn),
		users: make(map[message.UserID]map[message.ConnID]struct{}),
		subs:  make(map[message.Channel]map[message.ConnID]struct{}),
	}
}

// shutdown unsubscribes users from all channels and disconnects them.
func (h *clientHub) shutdown() {
	var wg sync.WaitGroup
	h.RLock()
	for _, user := range h.users {
		wg.Add(len(user))
		for uid := range user {
			cc, ok := h.conns[uid]
			if !ok {
				wg.Done()
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
	h.RUnlock()
	wg.Wait()
}

// add adds connection into clientHub connections registry.
func (h *clientHub) add(c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()
	user := c.user()

	h.conns[uid] = c

	_, ok := h.users[user]
	if !ok {
		h.users[user] = make(map[message.ConnID]struct{})
	}
	h.users[user][uid] = struct{}{}
	return nil
}

// remove removes connection from clientHub connections registry.
func (h *clientHub) remove(c clientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()
	user := c.user()

	delete(h.conns, uid)

	// try to find connection to delete, return early if not found.
	if _, ok := h.users[user]; !ok {
		return nil
	}
	if _, ok := h.users[user][uid]; !ok {
		return nil
	}

	// actually remove connection from hub.
	delete(h.users[user], uid)

	// clean up users map if it's needed.
	if len(h.users[user]) == 0 {
		delete(h.users, user)
	}

	return nil
}

// userConnections returns all connections of user with UserID in project.
func (h *clientHub) userConnections(user message.UserID) map[message.ConnID]clientConn {
	h.RLock()
	defer h.RUnlock()

	userConnections, ok := h.users[user]
	if !ok {
		return map[message.ConnID]clientConn{}
	}

	var conns map[message.ConnID]clientConn
	conns = make(map[message.ConnID]clientConn, len(userConnections))
	for uid := range userConnections {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
		conns[uid] = c
	}

	return conns
}

// addSub adds connection into clientHub subscriptions registry.
func (h *clientHub) addSub(ch message.Channel, c clientConn) (bool, error) {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	h.conns[uid] = c

	_, ok := h.subs[ch]
	if !ok {
		h.subs[ch] = make(map[message.ConnID]struct{})
	}
	h.subs[ch][uid] = struct{}{}
	if !ok {
		return true, nil
	}
	return false, nil
}

// removeSub removes connection from clientHub subscriptions registry.
func (h *clientHub) removeSub(ch message.Channel, c clientConn) (bool, error) {
	h.Lock()
	defer h.Unlock()

	uid := c.uid()

	// try to find subscription to delete, return early if not found.
	if _, ok := h.subs[ch]; !ok {
		return true, nil
	}
	if _, ok := h.subs[ch][uid]; !ok {
		return true, nil
	}

	// actually remove subscription from hub.
	delete(h.subs[ch], uid)

	// clean up subs map if it's needed.
	if len(h.subs[ch]) == 0 {
		delete(h.subs, ch)
		return true, nil
	}

	return false, nil
}

// broadcast sends message to all clients subscribed on channel.
func (h *clientHub) broadcast(ch message.Channel, message []byte) error {
	h.RLock()
	defer h.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subs[ch]
	if !ok {
		return nil
	}

	// iterate over them and send message individually
	for uid := range channelSubscriptions {
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

// nClients returns total number of client connections.
func (h *clientHub) nClients() int {
	h.RLock()
	defer h.RUnlock()
	total := 0
	for _, clientConnections := range h.users {
		total += len(clientConnections)
	}
	return total
}

// nUniqueClients returns a number of unique users connected.
func (h *clientHub) nUniqueClients() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.users)
}

// nChannels returns a total number of different channels.
func (h *clientHub) nChannels() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.subs)
}

// channels returns a slice of all active channels.
func (h *clientHub) channels() []message.Channel {
	h.RLock()
	defer h.RUnlock()
	channels := make([]message.Channel, len(h.subs))
	i := 0
	for ch := range h.subs {
		channels[i] = ch
		i++
	}
	return channels
}

// numSubscribers returns number of current subscribers for a given channel.
func (h *clientHub) numSubscribers(ch message.Channel) int {
	h.RLock()
	defer h.RUnlock()
	conns, ok := h.subs[ch]
	if !ok {
		return 0
	}
	return len(conns)
}

// adminHub manages admin connections from web interface.
type adminHub struct {
	sync.RWMutex

	// Registry to hold active admin connections.
	connections map[message.ConnID]adminConn
}

// newAdminHub initializes new adminHub.
func newAdminHub() *adminHub {
	return &adminHub{
		connections: make(map[message.ConnID]adminConn),
	}
}

// add adds connection to adminHub connections registry.
func (h *adminHub) add(c adminConn) error {
	h.Lock()
	defer h.Unlock()
	h.connections[c.uid()] = c
	return nil
}

// remove removes connection from adminHub connections registry.
func (h *adminHub) remove(c adminConn) error {
	h.Lock()
	defer h.Unlock()
	delete(h.connections, c.uid())
	return nil
}

// broadcast sends message to all connected admins.
func (h *adminHub) broadcast(message []byte) error {
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
