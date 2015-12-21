package libcentrifugo

import (
	"container/heap"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/priority"
)

// MemoryEngine allows to run Centrifugo without using Redis at all. All data managed inside process
// memory. With this engine you can only run single Centrifugo node. If you need to scale you should
// use Redis engine instead.
type MemoryEngine struct {
	app         *Application
	presenceHub *memoryPresenceHub
	historyHub  *memoryHistoryHub
}

// NewMemoryEngine initializes Memory Engine.
func NewMemoryEngine(app *Application) *MemoryEngine {
	e := &MemoryEngine{
		app:         app,
		presenceHub: newMemoryPresenceHub(),
		historyHub:  newMemoryHistoryHub(),
	}
	e.historyHub.initialize()
	return e
}

func (e *MemoryEngine) name() string {
	return "In memory – single node only"
}

func (e *MemoryEngine) run() error {
	return nil
}

func (e *MemoryEngine) publish(chID ChannelID, message []byte) (bool, error) {
	if !e.app.clients.hasSubscribers(chID) {
		return false, nil
	}
	return true, e.app.handleMsg(chID, message)
}

func (e *MemoryEngine) subscribe(chID ChannelID) error {
	return nil
}

func (e *MemoryEngine) unsubscribe(chID ChannelID) error {
	return nil
}

func (e *MemoryEngine) addPresence(chID ChannelID, uid ConnID, info ClientInfo) error {
	return e.presenceHub.add(chID, uid, info)
}

func (e *MemoryEngine) removePresence(chID ChannelID, uid ConnID) error {
	return e.presenceHub.remove(chID, uid)
}

func (e *MemoryEngine) presence(chID ChannelID) (map[ConnID]ClientInfo, error) {
	return e.presenceHub.get(chID)
}

func (e *MemoryEngine) addHistory(chID ChannelID, message Message, opts addHistoryOpts) error {
	return e.historyHub.add(chID, message, opts)
}

func (e *MemoryEngine) history(chID ChannelID, opts historyOpts) ([]Message, error) {
	return e.historyHub.get(chID, opts)
}

func (e *MemoryEngine) channels() ([]ChannelID, error) {
	return e.app.clients.channels(), nil
}

type memoryPresenceHub struct {
	sync.RWMutex
	presence map[ChannelID]map[ConnID]ClientInfo
}

func newMemoryPresenceHub() *memoryPresenceHub {
	return &memoryPresenceHub{
		presence: make(map[ChannelID]map[ConnID]ClientInfo),
	}
}

func (h *memoryPresenceHub) add(chID ChannelID, uid ConnID, info ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[chID]
	if !ok {
		h.presence[chID] = make(map[ConnID]ClientInfo)
	}
	h.presence[chID][uid] = info
	return nil
}

func (h *memoryPresenceHub) remove(chID ChannelID, uid ConnID) error {
	h.Lock()
	defer h.Unlock()

	if _, ok := h.presence[chID]; !ok {
		return nil
	}
	if _, ok := h.presence[chID][uid]; !ok {
		return nil
	}

	delete(h.presence[chID], uid)

	// clean up map if needed
	if len(h.presence[chID]) == 0 {
		delete(h.presence, chID)
	}

	return nil
}

func (h *memoryPresenceHub) get(chID ChannelID) (map[ConnID]ClientInfo, error) {
	h.RLock()
	defer h.RUnlock()

	presence, ok := h.presence[chID]
	if !ok {
		// return empty map
		return map[ConnID]ClientInfo{}, nil
	}

	var data map[ConnID]ClientInfo
	data = make(map[ConnID]ClientInfo, len(presence))
	for k, v := range presence {
		data[k] = v
	}
	return data, nil
}

type historyItem struct {
	messages []Message
	expireAt int64
}

func (i historyItem) isExpired() bool {
	return i.expireAt < time.Now().Unix()
}

type memoryHistoryHub struct {
	sync.RWMutex
	history   map[ChannelID]historyItem
	queue     priority.Queue
	nextCheck int64
}

func newMemoryHistoryHub() *memoryHistoryHub {
	return &memoryHistoryHub{
		history:   make(map[ChannelID]historyItem),
		queue:     priority.MakeQueue(),
		nextCheck: 0,
	}
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
			chID := ChannelID(item.Value)
			hItem, ok := h.history[chID]
			if !ok {
				continue
			}
			if hItem.expireAt <= expireAt {
				delete(h.history, chID)
			}
		}
		h.nextCheck = nextCheck
		h.Unlock()
	}
}

func (h *memoryHistoryHub) add(chID ChannelID, message Message, opts addHistoryOpts) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.history[chID]

	if opts.OnlySaveIfActive && !ok {
		// No active history for this channel so don't bother storing at all
		return nil
	}

	expireAt := time.Now().Unix() + int64(opts.Lifetime)
	heap.Push(&h.queue, &priority.Item{Value: string(chID), Priority: expireAt})
	if !ok {
		h.history[chID] = historyItem{
			messages: []Message{message},
			expireAt: expireAt,
		}
	} else {
		messages := h.history[chID].messages
		messages = append([]Message{message}, messages...)
		if len(messages) > opts.Size {
			messages = messages[0:opts.Size]
		}
		h.history[chID] = historyItem{
			messages: messages,
			expireAt: expireAt,
		}
	}

	if h.nextCheck == 0 || h.nextCheck > expireAt {
		h.nextCheck = expireAt
	}

	return nil
}

func (h *memoryHistoryHub) get(chID ChannelID, opts historyOpts) ([]Message, error) {
	h.RLock()
	defer h.RUnlock()

	hItem, ok := h.history[chID]
	if !ok {
		// return empty slice
		return []Message{}, nil
	}
	if hItem.isExpired() {
		// return empty slice
		delete(h.history, chID)
		return []Message{}, nil
	}
	if opts.Limit == 0 || opts.Limit >= len(hItem.messages) {
		return hItem.messages, nil
	} else {
		return hItem.messages[:opts.Limit], nil
	}
}
