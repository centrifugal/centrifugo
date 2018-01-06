package node

import (
	"sync"

	"github.com/centrifugal/centrifugo/lib/proto"
)

// Hub is an interface describing current client connections to node.
type Hub interface {
	Add(c Client) error
	Remove(c Client) error
	AddSub(ch string, c Client) (bool, error)
	RemoveSub(ch string, c Client) (bool, error)
	Broadcast(ch string, message []byte) error
	NumSubscribers(ch string) int
	NumClients() int
	NumUniqueClients() int
	NumChannels() int
	Channels() []string
	UserConnections(user string) map[string]Client
	Shutdown() error
}

// clientHub manages client connections.
type clientHub struct {
	sync.RWMutex

	// match ConnID with actual client connection.
	conns map[string]Client

	// registry to hold active client connections grouped by user.
	users map[string]map[string]struct{}

	// registry to hold active subscriptions of clients on channels.
	subs map[string]map[string]struct{}
}

// NewHub initializes clientHub.
func NewHub() Hub {
	return &clientHub{
		conns: make(map[string]Client),
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
	advice := proto.DisconnectShutdown
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
			go func(cc Client) {
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
func (h *clientHub) Add(c Client) error {
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
func (h *clientHub) Remove(c Client) error {
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
func (h *clientHub) UserConnections(user string) map[string]Client {
	h.RLock()
	defer h.RUnlock()

	userConnections, ok := h.users[user]
	if !ok {
		return map[string]Client{}
	}

	var conns map[string]Client
	conns = make(map[string]Client, len(userConnections))
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
func (h *clientHub) AddSub(ch string, c Client) (bool, error) {
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
func (h *clientHub) RemoveSub(ch string, c Client) (bool, error) {
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
