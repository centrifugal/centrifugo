package engine

import (
	"container/heap"
	"sync"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifugo/libcentrifugo/app"
	"github.com/centrifugal/centrifugo/libcentrifugo/config"
	"github.com/centrifugal/centrifugo/libcentrifugo/message"
	"github.com/centrifugal/centrifugo/libcentrifugo/priority"
)

// MemoryEngine allows to run Centrifugo without using Redis at all. All data managed inside process
// memory. With this engine you can only run single Centrifugo node. If you need to scale you should
// use Redis engine instead.
type MemoryEngine struct {
	app         app.App
	presenceHub *memoryPresenceHub
	historyHub  *memoryHistoryHub
}

// NewMemoryEngine initializes Memory Engine.
func NewMemoryEngine(application app.App) *MemoryEngine {
	e := &MemoryEngine{
		app:         application,
		presenceHub: newMemoryPresenceHub(),
		historyHub:  newMemoryHistoryHub(),
	}
	e.historyHub.initialize()
	return e
}

func (e *MemoryEngine) Name() string {
	return "In memory – single node only"
}

func (e *MemoryEngine) Run() error {
	return nil
}

func (e *MemoryEngine) PublishMessage(ch message.Channel, message *message.Message, opts *config.ChannelOptions) <-chan error {
	hasCurrentSubscribers := e.app.NumSubscribers(ch) > 0

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
	eChan <- e.app.ClientMsg(ch, message)
	return eChan
}

func (e *MemoryEngine) PublishJoin(ch message.Channel, message *message.JoinMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.app.JoinMsg(ch, message)
	return eChan
}

func (e *MemoryEngine) PublishLeave(ch message.Channel, message *message.LeaveMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.app.LeaveMsg(ch, message)
	return eChan
}

func (e *MemoryEngine) PublishControl(message *message.ControlMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.app.ControlMsg(message)
	return eChan
}

func (e *MemoryEngine) PublishAdmin(message *message.AdminMessage) <-chan error {
	eChan := make(chan error, 1)
	eChan <- e.app.AdminMsg(message)
	return eChan
}

func (e *MemoryEngine) Subscribe(ch message.Channel) error {
	return nil
}

func (e *MemoryEngine) Unsubscribe(ch message.Channel) error {
	return nil
}

func (e *MemoryEngine) AddPresence(ch message.Channel, uid message.ConnID, info message.ClientInfo) error {
	return e.presenceHub.add(ch, uid, info)
}

func (e *MemoryEngine) RemovePresence(ch message.Channel, uid message.ConnID) error {
	return e.presenceHub.remove(ch, uid)
}

func (e *MemoryEngine) Presence(ch message.Channel) (map[message.ConnID]message.ClientInfo, error) {
	return e.presenceHub.get(ch)
}

func (e *MemoryEngine) History(ch message.Channel, limit int) ([]message.Message, error) {
	return e.historyHub.get(ch, limit)
}

func (e *MemoryEngine) Channels() ([]message.Channel, error) {
	return e.app.Channels(), nil
}

type memoryPresenceHub struct {
	sync.RWMutex
	presence map[message.Channel]map[message.ConnID]message.ClientInfo
}

func newMemoryPresenceHub() *memoryPresenceHub {
	return &memoryPresenceHub{
		presence: make(map[message.Channel]map[message.ConnID]message.ClientInfo),
	}
}

func (h *memoryPresenceHub) add(ch message.Channel, uid message.ConnID, info message.ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[ch]
	if !ok {
		h.presence[ch] = make(map[message.ConnID]message.ClientInfo)
	}
	h.presence[ch][uid] = info
	return nil
}

func (h *memoryPresenceHub) remove(ch message.Channel, uid message.ConnID) error {
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

func (h *memoryPresenceHub) get(ch message.Channel) (map[message.ConnID]message.ClientInfo, error) {
	h.RLock()
	defer h.RUnlock()

	presence, ok := h.presence[ch]
	if !ok {
		// return empty map
		return map[message.ConnID]message.ClientInfo{}, nil
	}

	var data map[message.ConnID]message.ClientInfo
	data = make(map[message.ConnID]message.ClientInfo, len(presence))
	for k, v := range presence {
		data[k] = v
	}
	return data, nil
}

type historyItem struct {
	messages []message.Message
	expireAt int64
}

func (i historyItem) isExpired() bool {
	return i.expireAt < time.Now().Unix()
}

type memoryHistoryHub struct {
	sync.RWMutex
	history   map[message.Channel]historyItem
	queue     priority.Queue
	nextCheck int64
}

func newMemoryHistoryHub() *memoryHistoryHub {
	return &memoryHistoryHub{
		history:   make(map[message.Channel]historyItem),
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
			ch := message.Channel(item.Value)
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

func (h *memoryHistoryHub) add(ch message.Channel, msg message.Message, opts addHistoryOpts) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.history[ch]

	if opts.DropInactive && !ok {
		// No active history for this channel so don't bother storing at all
		return nil
	}

	expireAt := time.Now().Unix() + int64(opts.Lifetime)
	heap.Push(&h.queue, &priority.Item{Value: string(ch), Priority: expireAt})
	if !ok {
		h.history[ch] = historyItem{
			messages: []message.Message{msg},
			expireAt: expireAt,
		}
	} else {
		messages := h.history[ch].messages
		messages = append([]message.Message{msg}, messages...)
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

func (h *memoryHistoryHub) get(ch message.Channel, limit int) ([]message.Message, error) {
	h.RLock()
	defer h.RUnlock()

	hItem, ok := h.history[ch]
	if !ok {
		// return empty slice
		return []message.Message{}, nil
	}
	if hItem.isExpired() {
		// return empty slice
		delete(h.history, ch)
		return []message.Message{}, nil
	}
	if limit == 0 || limit >= len(hItem.messages) {
		return hItem.messages, nil
	} else {
		return hItem.messages[:limit], nil
	}
}
