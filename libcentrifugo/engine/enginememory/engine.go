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
	plugin.RegisterEngine("memory", MemoryEnginePlugin)
}

// MemoryEngine allows to run Centrifugo without using Redis at all.
// All data managed inside process memory. With this engine you can
// only run single Centrifugo node. If you need to scale you should
// use Redis engine instead.
type MemoryEngine struct {
	node        *node.Node
	presenceHub *memoryPresenceHub
	historyHub  *memoryHistoryHub
}

type MemoryEngineConfig struct{}

func MemoryEnginePlugin(n *node.Node, c config.Getter) (engine.Engine, error) {
	return NewMemoryEngine(n, &MemoryEngineConfig{})
}

// NewMemoryEngine initializes Memory Engine.
func NewMemoryEngine(n *node.Node, conf *MemoryEngineConfig) (engine.Engine, error) {
	e := &MemoryEngine{
		node:        n,
		presenceHub: newMemoryPresenceHub(),
		historyHub:  newMemoryHistoryHub(),
	}
	e.historyHub.initialize()
	return e, nil
}

func (e *MemoryEngine) Name() string {
	return "In memory – single node only"
}

func (e *MemoryEngine) Run() error {
	return nil
}

func (e *MemoryEngine) Shutdown() error {
	return errors.New("Shutdown not implemented")
}

func (e *MemoryEngine) PublishMessage(message *proto.Message, opts *proto.ChannelOptions) <-chan error {

	ch := message.Channel

	hasCurrentSubscribers := e.node.ClientHub().NumSubscribers(ch) > 0

	if opts != nil && opts.HistorySize > 0 && opts.HistoryLifetime > 0 {
		histOpts := addHistoryOpts{
			Size:         opts.HistorySize,
			Lifetime:     opts.HistoryLifetime,
			DropInactive: (opts.HistoryDropInactive && !hasCurrentSubscribers),
		}
		err := e.historyHub.add(ch, *message, histOpts)
		if err != nil {
			logger.ERROR.Println(err)
		}
	}

	eChan := make(chan error, 1)
	eChan <- e.node.ClientMsg(message)
	return eChan
}

func (e *MemoryEngine) PublishJoin(message *proto.JoinMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.JoinMsg(message)
	return eChan
}

func (e *MemoryEngine) PublishLeave(message *proto.LeaveMessage, opts *proto.ChannelOptions) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.LeaveMsg(message)
	return eChan
}

func (e *MemoryEngine) PublishControl(message *proto.ControlMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.ControlMsg(message)
	return eChan
}

func (e *MemoryEngine) PublishAdmin(message *proto.AdminMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.node.AdminMsg(message)
	return eChan
}

func (e *MemoryEngine) Subscribe(ch string) error {
	return nil
}

func (e *MemoryEngine) Unsubscribe(ch string) error {
	return nil
}

func (e *MemoryEngine) AddPresence(ch string, uid string, info proto.ClientInfo, expire int) error {
	return e.presenceHub.add(ch, uid, info)
}

func (e *MemoryEngine) RemovePresence(ch string, uid string) error {
	return e.presenceHub.remove(ch, uid)
}

func (e *MemoryEngine) Presence(ch string) (map[string]proto.ClientInfo, error) {
	return e.presenceHub.get(ch)
}

func (e *MemoryEngine) History(ch string, limit int) ([]proto.Message, error) {
	return e.historyHub.get(ch, limit)
}

func (e *MemoryEngine) Channels() ([]string, error) {
	return e.node.ClientHub().Channels(), nil
}

type memoryPresenceHub struct {
	sync.RWMutex
	presence map[string]map[string]proto.ClientInfo
}

func newMemoryPresenceHub() *memoryPresenceHub {
	return &memoryPresenceHub{
		presence: make(map[string]map[string]proto.ClientInfo),
	}
}

func (h *memoryPresenceHub) add(ch string, uid string, info proto.ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[ch]
	if !ok {
		h.presence[ch] = make(map[string]proto.ClientInfo)
	}
	h.presence[ch][uid] = info
	return nil
}

func (h *memoryPresenceHub) remove(ch string, uid string) error {
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

func (h *memoryPresenceHub) get(ch string) (map[string]proto.ClientInfo, error) {
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

type memoryHistoryHub struct {
	sync.RWMutex
	history   map[string]historyItem
	queue     priority.Queue
	nextCheck int64
}

func newMemoryHistoryHub() *memoryHistoryHub {
	return &memoryHistoryHub{
		history:   make(map[string]historyItem),
		queue:     priority.MakeQueue(),
		nextCheck: 0,
	}
}

type addHistoryOpts struct {
	// Size is maximum size of channel history that engine must maintain.
	Size int
	// Lifetime is maximum amount of seconds history messages should exist
	// before expiring and most probably being deleted (to prevent memory leaks).
	Lifetime int
	// DropInactive hints to the engine that there were no actual subscribers
	// connected when message was published, and that it can skip saving if there is
	// no unexpired history for the channel (i.e. no subscribers active within history_lifetime)
	DropInactive bool
}

func (h *memoryHistoryHub) initialize() {
	go h.expire()
}

func (h *memoryHistoryHub) expire() {
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

func (h *memoryHistoryHub) add(ch string, msg proto.Message, opts addHistoryOpts) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.history[ch]

	if opts.DropInactive && !ok {
		// No active history for this channel so don't bother storing at all
		return nil
	}

	expireAt := time.Now().Unix() + int64(opts.Lifetime)
	heap.Push(&h.queue, &priority.Item{Value: ch, Priority: expireAt})
	if !ok {
		h.history[ch] = historyItem{
			messages: []proto.Message{msg},
			expireAt: expireAt,
		}
	} else {
		messages := h.history[ch].messages
		messages = append([]proto.Message{msg}, messages...)
		if len(messages) > opts.Size {
			messages = messages[0:opts.Size]
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

func (h *memoryHistoryHub) get(ch string, limit int) ([]proto.Message, error) {
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
	} else {
		return hItem.messages[:limit], nil
	}
}
