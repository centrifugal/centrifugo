package enginememory

import (
	"container/heap"
	"errors"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/centrifugal/centrifugo/libcentrifugo/plugin"
	"github.com/centrifugal/centrifugo/libcentrifugo/priority"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

func init() {
	plugin.RegisterEngine("memory", Plugin)
}

// Plugin returns new memory engine.
func Plugin(n *node.Node, c config.Getter) (engine.Engine, error) {
	return New(n, &Config{})
}

// MemoryEngine allows to run Centrifugo without using Redis at all.
// All data managed inside process memory. With this engine you can
// only run single Centrifugo node. If you need to scale you should
// use Redis engine instead.
type MemoryEngine struct {
	node        *node.Node
	presenceHub *presenceHub
	historyHub  *historyHub
}

// Config is a memory engine congig struct.
type Config struct{}

// New initializes Memory Engine.
func New(n *node.Node, conf *Config) (*MemoryEngine, error) {
	e := &MemoryEngine{
		node:        n,
		presenceHub: newPresenceHub(),
		historyHub:  newHistoryHub(),
	}
	e.historyHub.initialize()
	return e, nil
}

// Name returns a name of engine.
func (e *MemoryEngine) Name() string {
	return "In memory – single node only"
}

// Run runs memory engine - we do not have any logic here as Memory Engine ready to work
// just after initialization.
func (e *MemoryEngine) Run() error {
	return nil
}

// Shutdown shuts down engine.
func (e *MemoryEngine) Shutdown() error {
	return errors.New("Shutdown not implemented")
}

// PublishMessage adds message into history hub and calls node ClientMsg method to handle message.
// We don't have any PUB/SUB here as Memory Engine is single node only.
func (e *MemoryEngine) PublishMessage(message *proto.Message, opts *proto.ChannelOptions) <-chan error {

	ch := message.Channel

	hasCurrentSubscribers := e.node.ClientHub().NumSubscribers(ch) > 0

	if opts != nil && opts.HistorySize > 0 && opts.HistoryLifetime > 0 {
		err := e.historyHub.add(ch, *message, opts, hasCurrentSubscribers)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	eChan := make(chan error, 1)
	eChan <- e.node.ClientMsg(message)
	return eChan
}

// PublishJoin - see Engine interface description.
func (e *MemoryEngine) PublishJoin(message *proto.JoinMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.JoinMsg(message)
	return eChan
}

// PublishLeave - see Engine interface description.
func (e *MemoryEngine) PublishLeave(message *proto.LeaveMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.LeaveMsg(message)
	return eChan
}

// PublishControl - see Engine interface description.
func (e *MemoryEngine) PublishControl(message *proto.ControlMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.ControlMsg(message)
	return eChan
}

// PublishAdmin - see Engine interface description.
func (e *MemoryEngine) PublishAdmin(message *proto.AdminMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.AdminMsg(message)
	return eChan
}

// Subscribe is noop here.
func (e *MemoryEngine) Subscribe(ch string) error {
	return nil
}

// Unsubscribe node from channel.
// In case of memory engine its only job is to touch channel history for history lifetime period.
func (e *MemoryEngine) Unsubscribe(ch string) error {
	if chOpts, err := e.node.ChannelOpts(ch); err == nil && chOpts.HistoryDropInactive {
		e.historyHub.touch(ch, &chOpts)
	}
	return nil
}

// AddPresence adds client info into presence hub.
func (e *MemoryEngine) AddPresence(ch string, uid string, info proto.ClientInfo, expire int) error {
	return e.presenceHub.add(ch, uid, info)
}

// RemovePresence removes client info from presence hub.
func (e *MemoryEngine) RemovePresence(ch string, uid string) error {
	return e.presenceHub.remove(ch, uid)
}

// Presence extracts presence info from presence hub.
func (e *MemoryEngine) Presence(ch string) (map[string]proto.ClientInfo, error) {
	return e.presenceHub.get(ch)
}

// History extracts history from history hub.
func (e *MemoryEngine) History(ch string, limit int) ([]proto.Message, error) {
	return e.historyHub.get(ch, limit)
}

// Channels returns all channels node currently subscribed on.
func (e *MemoryEngine) Channels() ([]string, error) {
	return e.node.ClientHub().Channels(), nil
}

type presenceHub struct {
	sync.RWMutex
	presence map[string]map[string]proto.ClientInfo
}

func newPresenceHub() *presenceHub {
	return &presenceHub{
		presence: make(map[string]map[string]proto.ClientInfo),
	}
}

func (h *presenceHub) add(ch string, uid string, info proto.ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[ch]
	if !ok {
		h.presence[ch] = make(map[string]proto.ClientInfo)
	}
	h.presence[ch][uid] = info
	return nil
}

func (h *presenceHub) remove(ch string, uid string) error {
	h.Lock()
	defer h.Unlock()

	if _, ok := h.presence[ch]; !ok {
		return nil
	}
	if _, ok := h.presence[ch][uid]; !ok {
		return nil
	}

	delete(h.presence[ch], uid)

	// clean up map if needed
	if len(h.presence[ch]) == 0 {
		delete(h.presence, ch)
	}

	return nil
}

func (h *presenceHub) get(ch string) (map[string]proto.ClientInfo, error) {
	h.RLock()
	defer h.RUnlock()

	presence, ok := h.presence[ch]
	if !ok {
		// return empty map
		return map[string]proto.ClientInfo{}, nil
	}

	var data map[string]proto.ClientInfo
	data = make(map[string]proto.ClientInfo, len(presence))
	for k, v := range presence {
		data[k] = v
	}
	return data, nil
}

type historyItem struct {
	messages []proto.Message
	expireAt int64
}

func (i historyItem) isExpired() bool {
	return i.expireAt < time.Now().Unix()
}

type historyHub struct {
	sync.RWMutex
	history   map[string]historyItem
	queue     priority.Queue
	nextCheck int64
}

func newHistoryHub() *historyHub {
	return &historyHub{
		history:   make(map[string]historyItem),
		queue:     priority.MakeQueue(),
		nextCheck: 0,
	}
}

func (h *historyHub) initialize() {
	go h.expire()
}

func (h *historyHub) expire() {
	var nextCheck int64
	for {
		time.Sleep(time.Second)
		h.Lock()
		if h.nextCheck == 0 || h.nextCheck > time.Now().Unix() {
			h.Unlock()
			continue
		}
		nextCheck = 0
		for h.queue.Len() > 0 {
			item := heap.Pop(&h.queue).(*priority.Item)
			expireAt := item.Priority
			if expireAt > time.Now().Unix() {
				heap.Push(&h.queue, item)
				nextCheck = expireAt
				break
			}
			ch := item.Value
			hItem, ok := h.history[ch]
			if !ok {
				continue
			}
			if hItem.expireAt <= expireAt {
				delete(h.history, ch)
			}
		}
		h.nextCheck = nextCheck
		h.Unlock()
	}
}

func (h *historyHub) touch(ch string, opts *proto.ChannelOptions) {
	h.Lock()
	defer h.Unlock()

	item, ok := h.history[ch]
	expireAt := time.Now().Unix() + int64(opts.HistoryLifetime)

	heap.Push(&h.queue, &priority.Item{Value: ch, Priority: expireAt})

	if !ok {
		h.history[ch] = historyItem{
			messages: []proto.Message{},
			expireAt: expireAt,
		}
	} else {
		item.expireAt = expireAt
	}

	if h.nextCheck == 0 || h.nextCheck > expireAt {
		h.nextCheck = expireAt
	}
}

func (h *historyHub) add(ch string, msg proto.Message, opts *proto.ChannelOptions, hasSubscribers bool) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.history[ch]

	if opts.HistoryDropInactive && !hasSubscribers && !ok {
		// No active history for this channel so don't bother storing at all
		return nil
	}

	expireAt := time.Now().Unix() + int64(opts.HistoryLifetime)
	heap.Push(&h.queue, &priority.Item{Value: ch, Priority: expireAt})
	if !ok {
		h.history[ch] = historyItem{
			messages: []proto.Message{msg},
			expireAt: expireAt,
		}
	} else {
		messages := h.history[ch].messages
		messages = append([]proto.Message{msg}, messages...)
		if len(messages) > opts.HistorySize {
			messages = messages[0:opts.HistorySize]
		}
		h.history[ch] = historyItem{
			messages: messages,
			expireAt: expireAt,
		}
	}

	if h.nextCheck == 0 || h.nextCheck > expireAt {
		h.nextCheck = expireAt
	}

	return nil
}

func (h *historyHub) get(ch string, limit int) ([]proto.Message, error) {
	h.RLock()
	defer h.RUnlock()

	hItem, ok := h.history[ch]
	if !ok {
		// return empty slice
		return []proto.Message{}, nil
	}
	if hItem.isExpired() {
		// return empty slice
		delete(h.history, ch)
		return []proto.Message{}, nil
	}
	if limit == 0 || limit >= len(hItem.messages) {
		return hItem.messages, nil
	}
	return hItem.messages[:limit], nil
}
