package conns

import (
	"sync"

	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
)

// ClientHub is an interface describing current client connections to node.
type ClientHub interface {
	Add(c ClientConn) error
	Remove(c ClientConn) error
	AddSub(ch string, c ClientConn) (bool, error)
	RemoveSub(ch string, c ClientConn) (bool, error)
	Broadcast(ch string, message []byte) error
	NumSubscribers(ch string) int
	NumClients() int
	NumUniqueClients() int
	NumChannels() int
	Channels() []string
	UserConnections(user string) map[string]ClientConn
	Shutdown() error
}

// clientHub manages client connections.
type clientHub struct {
	sync.RWMutex

	// match ConnID with actual client connection.
	conns map[string]ClientConn

	// registry to hold active client connections grouped by user.
	users map[string]map[string]struct{}

	// registry to hold active subscriptions of clients on channels.
	subs map[string]map[string]struct{}
}

// NewClientHub initializes clientHub.
func NewClientHub() ClientHub {
	return &clientHub{
		conns: make(map[string]ClientConn),
		users: make(map[string]map[string]struct{}),
		subs:  make(map[string]map[string]struct{}),
	}
}

var (
	// ShutdownSemaphoreChanBufferSize limits graceful disconnects concurrency on
	// node shutdown.
	ShutdownSemaphoreChanBufferSize = 1000
)

// Shutdown unsubscribes users from all channels and disconnects them.
func (h *clientHub) Shutdown() error {
	var wg sync.WaitGroup
	h.RLock()
	advice := &DisconnectAdvice{Reason: "shutting down", Reconnect: true}
	// Limit concurrency here to prevent memory burst on shutdown.
	sem := make(chan struct{}, ShutdownSemaphoreChanBufferSize)
	for _, user := range h.users {
		wg.Add(len(user))
		for uid := range user {
			cc, ok := h.conns[uid]
			if !ok {
				wg.Done()
				continue
			}
			sem <- struct{}{}
			go func(cc ClientConn) {
				defer func() { <-sem }()
				for _, ch := range cc.Channels() {
					cc.Unsubscribe(ch)
				}
				cc.Close(advice)
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
		h.users[user] = make(map[string]struct{})
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
func (h *clientHub) UserConnections(user string) map[string]ClientConn {
	h.RLock()
	defer h.RUnlock()

	userConnections, ok := h.users[user]
	if !ok {
		return map[string]ClientConn{}
	}

	var conns map[string]ClientConn
	conns = make(map[string]ClientConn, len(userConnections))
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
func (h *clientHub) AddSub(ch string, c ClientConn) (bool, error) {
	h.Lock()
	defer h.Unlock()

	uid := c.UID()

	h.conns[uid] = c

	_, ok := h.subs[ch]
	if !ok {
		h.subs[ch] = make(map[string]struct{})
	}
	h.subs[ch][uid] = struct{}{}
	if !ok {
		return true, nil
	}
	return false, nil
}

// RemoveSub removes connection from clientHub subscriptions registry.
func (h *clientHub) RemoveSub(ch string, c ClientConn) (bool, error) {
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
func (h *clientHub) Broadcast(ch string, message []byte) error {
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
		c.Send(message)
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
func (h *clientHub) Channels() []string {
	h.RLock()
	defer h.RUnlock()
	channels := make([]string, len(h.subs))
	i := 0
	for ch := range h.subs {
		channels[i] = ch
		i++
	}
	return channels
}

// NumSubscribers returns number of current subscribers for a given channel.
func (h *clientHub) NumSubscribers(ch string) int {
	h.RLock()
	defer h.RUnlock()
	conns, ok := h.subs[ch]
	if !ok {
		return 0
	}
	return len(conns)
}

// AdminHub is an interface describing admin connections to node.
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
	connections map[string]AdminConn
}

// NewAdminHub initializes new adminHub.
func NewAdminHub() AdminHub {
	return &adminHub{
		connections: make(map[string]AdminConn),
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
