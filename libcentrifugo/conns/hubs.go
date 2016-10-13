package conns

import (
	"sync"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

type ClientHub interface {
	Add(c ClientConn) error
	Remove(c ClientConn) error
	AddSub(ch proto.Channel, c ClientConn) (bool, error)
	RemoveSub(ch proto.Channel, c ClientConn) (bool, error)
	Broadcast(ch proto.Channel, message []byte) error
	NumSubscribers(ch proto.Channel) int
	NumClients() int
	NumUniqueClients() int
	NumChannels() int
	Channels() []proto.Channel
	UserConnections(user proto.UserID) map[proto.ConnID]ClientConn
	Shutdown() error
}

// clientHub manages client connections.
type clientHub struct {
	sync.RWMutex

	// match ConnID with actual client connection.
	conns map[proto.ConnID]ClientConn

	// registry to hold active client connections grouped by UserID.
	users map[proto.UserID]map[proto.ConnID]struct{}

	// registry to hold active subscriptions of clients on channels.
	subs map[proto.Channel]map[proto.ConnID]struct{}
}

// newClientHub initializes clientHub.
func NewClientHub() ClientHub {
	return &clientHub{
		conns: make(map[proto.ConnID]ClientConn),
		users: make(map[proto.UserID]map[proto.ConnID]struct{}),
		subs:  make(map[proto.Channel]map[proto.ConnID]struct{}),
	}
}

// Shutdown unsubscribes users from all channels and disconnects them.
func (h *clientHub) Shutdown() error {
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
			go func(cc ClientConn) {
				for _, ch := range cc.Channels() {
					cc.Unsubscribe(ch)
				}
				cc.Close("shutting down")
				wg.Done()
			}(cc)
		}
	}
	h.RUnlock()
	wg.Wait()
	return nil
}

// Add adds connection into clientHub connections registry.
func (h *clientHub) Add(c ClientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.UID()
	user := c.User()

	h.conns[uid] = c

	_, ok := h.users[user]
	if !ok {
		h.users[user] = make(map[proto.ConnID]struct{})
	}
	h.users[user][uid] = struct{}{}
	return nil
}

// Remove removes connection from clientHub connections registry.
func (h *clientHub) Remove(c ClientConn) error {
	h.Lock()
	defer h.Unlock()

	uid := c.UID()
	user := c.User()

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

// userConnections returns all connections of user with specified UserID.
func (h *clientHub) UserConnections(user proto.UserID) map[proto.ConnID]ClientConn {
	h.RLock()
	defer h.RUnlock()

	userConnections, ok := h.users[user]
	if !ok {
		return map[proto.ConnID]ClientConn{}
	}

	var conns map[proto.ConnID]ClientConn
	conns = make(map[proto.ConnID]ClientConn, len(userConnections))
	for uid := range userConnections {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
		conns[uid] = c
	}

	return conns
}

// AddSub adds connection into clientHub subscriptions registry.
func (h *clientHub) AddSub(ch proto.Channel, c ClientConn) (bool, error) {
	h.Lock()
	defer h.Unlock()

	uid := c.UID()

	h.conns[uid] = c

	_, ok := h.subs[ch]
	if !ok {
		h.subs[ch] = make(map[proto.ConnID]struct{})
	}
	h.subs[ch][uid] = struct{}{}
	if !ok {
		return true, nil
	}
	return false, nil
}

// RemoveSub removes connection from clientHub subscriptions registry.
func (h *clientHub) RemoveSub(ch proto.Channel, c ClientConn) (bool, error) {
	h.Lock()
	defer h.Unlock()

	uid := c.UID()

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

// Broadcast sends message to all clients subscribed on channel.
func (h *clientHub) Broadcast(ch proto.Channel, message []byte) error {
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
		err := c.Send(message)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}
	return nil
}

// NumClients returns total number of client connections.
func (h *clientHub) NumClients() int {
	h.RLock()
	defer h.RUnlock()
	total := 0
	for _, clientConnections := range h.users {
		total += len(clientConnections)
	}
	return total
}

// NumUniqueClients returns a number of unique users connected.
func (h *clientHub) NumUniqueClients() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.users)
}

// NumChannels returns a total number of different channels.
func (h *clientHub) NumChannels() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.subs)
}

// Channels returns a slice of all active channels.
func (h *clientHub) Channels() []proto.Channel {
	h.RLock()
	defer h.RUnlock()
	channels := make([]proto.Channel, len(h.subs))
	i := 0
	for ch := range h.subs {
		channels[i] = ch
		i++
	}
	return channels
}

// NumSubscribers returns number of current subscribers for a given channel.
func (h *clientHub) NumSubscribers(ch proto.Channel) int {
	h.RLock()
	defer h.RUnlock()
	conns, ok := h.subs[ch]
	if !ok {
		return 0
	}
	return len(conns)
}

type AdminHub interface {
	Add(c AdminConn) error
	Remove(c AdminConn) error
	NumAdmins() int
	Broadcast(message []byte) error
	Shutdown() error
}

// adminHub manages admin connections from web interface.
type adminHub struct {
	sync.RWMutex

	// Registry to hold active admin connections.
	connections map[proto.ConnID]AdminConn
}

// newAdminHub initializes new adminHub.
func NewAdminHub() AdminHub {
	return &adminHub{
		connections: make(map[proto.ConnID]AdminConn),
	}
}

// add adds connection to adminHub connections registry.
func (h *adminHub) Add(c AdminConn) error {
	h.Lock()
	defer h.Unlock()
	h.connections[c.UID()] = c
	return nil
}

// remove removes connection from adminHub connections registry.
func (h *adminHub) Remove(c AdminConn) error {
	h.Lock()
	defer h.Unlock()
	delete(h.connections, c.UID())
	return nil
}

// broadcast sends message to all connected admins.
func (h *adminHub) Broadcast(message []byte) error {
	h.RLock()
	defer h.RUnlock()
	for _, c := range h.connections {
		err := c.Send(message)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}
	return nil
}

// NumAdmins.
func (h *adminHub) NumAdmins() int {
	h.RLock()
	defer h.RUnlock()
	return len(h.connections)
}

// Shutdown.
func (h *adminHub) Shutdown() error {
	return nil
}
