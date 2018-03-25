package centrifuge

import (
	"sync"

	"github.com/centrifugal/centrifuge/internal/proto"
)

// Hub manages client connections.
type Hub struct {
	mu sync.RWMutex

	// match ConnID with actual client connection.
	conns map[string]*client

	// registry to hold active client connections grouped by user.
	users map[string]map[string]struct{}

	// registry to hold active subscriptions of clients on channels.
	subs map[string]map[string]struct{}
}

// newHub initializes Hub.
func newHub() *Hub {
	return &Hub{
		conns: make(map[string]*client),
		users: make(map[string]map[string]struct{}),
		subs:  make(map[string]map[string]struct{}),
	}
}

var (
	// hubShutdownSemaphoreSize limits graceful disconnects concurrency on
	// node shutdown.
	hubShutdownSemaphoreSize = 1000
)

// shutdown unsubscribes users from all channels and disconnects them.
func (h *Hub) shutdown() error {
	var wg sync.WaitGroup
	h.mu.RLock()
	advice := DisconnectShutdown
	// Limit concurrency here to prevent memory burst on shutdown.
	sem := make(chan struct{}, hubShutdownSemaphoreSize)
	for _, user := range h.users {
		wg.Add(len(user))
		for uid := range user {
			cc, ok := h.conns[uid]
			if !ok {
				wg.Done()
				continue
			}
			sem <- struct{}{}
			go func(cc *client) {
				defer func() { <-sem }()
				cc.Close(advice)
				wg.Done()
			}(cc)
		}
	}
	h.mu.RUnlock()
	wg.Wait()
	return nil
}

func (h *Hub) disconnect(user string, reconnect bool) error {
	userConnections := h.userConnections(user)
	advice := &Disconnect{Reason: "disconnect", Reconnect: reconnect}
	for _, c := range userConnections {
		go func(cc *client) {
			cc.Close(advice)
		}(c)
	}
	return nil
}

func (h *Hub) unsubscribe(user string, ch string) error {
	userConnections := h.userConnections(user)
	for _, c := range userConnections {
		var channels []string
		if string(ch) == "" {
			// unsubscribe from all channels
			channels = c.Channels()
		} else {
			channels = []string{ch}
		}

		for _, channel := range channels {
			err := c.Unsubscribe(channel)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// add adds connection into clientHub connections registry.
func (h *Hub) add(c *client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()
	user := c.UserID()

	h.conns[uid] = c

	_, ok := h.users[user]
	if !ok {
		h.users[user] = make(map[string]struct{})
	}
	h.users[user][uid] = struct{}{}
	return nil
}

// Remove removes connection from clientHub connections registry.
func (h *Hub) remove(c *client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()
	user := c.UserID()

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
func (h *Hub) userConnections(userID string) map[string]*client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userConnections, ok := h.users[userID]
	if !ok {
		return map[string]*client{}
	}

	var conns map[string]*client
	conns = make(map[string]*client, len(userConnections))
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
func (h *Hub) addSub(ch string, c *client) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()

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

// removeSub removes connection from clientHub subscriptions registry.
func (h *Hub) removeSub(ch string, c *client) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uid := c.ID()

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
func (h *Hub) broadcastPublication(channel string, publication *proto.Publication) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subs[channel]
	if !ok {
		return nil
	}

	var jsonReply *preparedReply
	var protobufReply *preparedReply

	// iterate over them and send message individually
	for uid := range channelSubscriptions {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
		enc := c.Transport().Encoding()
		if enc == proto.EncodingJSON {
			if jsonReply == nil {
				data, err := proto.GetMessageEncoder(enc).EncodePublication(publication)
				if err != nil {
					return err
				}
				messageBytes, err := proto.GetMessageEncoder(enc).Encode(proto.NewPublicationMessage(channel, data))
				if err != nil {
					return err
				}
				reply := &proto.Reply{
					Result: messageBytes,
				}
				jsonReply = newPreparedReply(reply, proto.EncodingJSON)
			}
			c.transport.Send(jsonReply)
		} else if enc == proto.EncodingProtobuf {
			if protobufReply == nil {
				data, err := proto.GetMessageEncoder(enc).EncodePublication(publication)
				if err != nil {
					return err
				}
				messageBytes, err := proto.GetMessageEncoder(enc).Encode(proto.NewPublicationMessage(channel, data))
				if err != nil {
					return err
				}
				reply := &proto.Reply{
					Result: messageBytes,
				}
				protobufReply = newPreparedReply(reply, proto.EncodingProtobuf)
			}
			c.transport.Send(protobufReply)
		}
	}
	return nil
}

// broadcastJoin sends message to all clients subscribed on channel.
func (h *Hub) broadcastJoin(channel string, join *proto.Join) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subs[channel]
	if !ok {
		return nil
	}

	var jsonReply *preparedReply
	var protobufReply *preparedReply

	// iterate over them and send message individually
	for uid := range channelSubscriptions {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
		enc := c.Transport().Encoding()
		if enc == proto.EncodingJSON {
			if jsonReply == nil {
				data, err := proto.GetMessageEncoder(enc).EncodeJoin(join)
				if err != nil {
					return err
				}
				messageBytes, err := proto.GetMessageEncoder(enc).Encode(proto.NewJoinMessage(channel, data))
				if err != nil {
					return err
				}
				reply := &proto.Reply{
					Result: messageBytes,
				}
				jsonReply = newPreparedReply(reply, proto.EncodingJSON)
			}
			c.transport.Send(jsonReply)
		} else if enc == proto.EncodingProtobuf {
			if protobufReply == nil {
				data, err := proto.GetMessageEncoder(enc).EncodeJoin(join)
				if err != nil {
					return err
				}
				messageBytes, err := proto.GetMessageEncoder(enc).Encode(proto.NewJoinMessage(channel, data))
				if err != nil {
					return err
				}
				reply := &proto.Reply{
					Result: messageBytes,
				}
				protobufReply = newPreparedReply(reply, proto.EncodingProtobuf)
			}
			c.transport.Send(protobufReply)
		}
	}
	return nil
}

// broadcastLeave sends message to all clients subscribed on channel.
func (h *Hub) broadcastLeave(channel string, leave *proto.Leave) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// get connections currently subscribed on channel
	channelSubscriptions, ok := h.subs[channel]
	if !ok {
		return nil
	}

	var jsonReply *preparedReply
	var protobufReply *preparedReply

	// iterate over them and send message individually
	for uid := range channelSubscriptions {
		c, ok := h.conns[uid]
		if !ok {
			continue
		}
		enc := c.Transport().Encoding()
		if enc == proto.EncodingJSON {
			if jsonReply == nil {
				data, err := proto.GetMessageEncoder(enc).EncodeLeave(leave)
				if err != nil {
					return err
				}
				messageBytes, err := proto.GetMessageEncoder(enc).Encode(proto.NewLeaveMessage(channel, data))
				if err != nil {
					return err
				}
				reply := &proto.Reply{
					Result: messageBytes,
				}
				jsonReply = newPreparedReply(reply, proto.EncodingJSON)
			}
			c.transport.Send(jsonReply)
		} else if enc == proto.EncodingProtobuf {
			if protobufReply == nil {
				data, err := proto.GetMessageEncoder(enc).EncodeLeave(leave)
				if err != nil {
					return err
				}
				messageBytes, err := proto.GetMessageEncoder(enc).Encode(proto.NewLeaveMessage(channel, data))
				if err != nil {
					return err
				}
				reply := &proto.Reply{
					Result: messageBytes,
				}
				protobufReply = newPreparedReply(reply, proto.EncodingProtobuf)
			}
			c.transport.Send(protobufReply)
		}
	}
	return nil
}

// NumClients returns total number of client connections.
func (h *Hub) NumClients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, clientConnections := range h.users {
		total += len(clientConnections)
	}
	return total
}

// NumUsers returns a number of unique users connected.
func (h *Hub) NumUsers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users)
}

// NumChannels returns a total number of different channels.
func (h *Hub) NumChannels() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// Channels returns a slice of all active channels.
func (h *Hub) Channels() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	channels := make([]string, len(h.subs))
	i := 0
	for ch := range h.subs {
		channels[i] = ch
		i++
	}
	return channels
}

// NumSubscribers returns number of current subscribers for a given channel.
func (h *Hub) NumSubscribers(ch string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.subs[ch]
	if !ok {
		return 0
	}
	return len(conns)
}
