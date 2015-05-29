package libcentrifugo

import (
	"container/heap"
	"sync"
	"time"

	"github.com/centrifugal/centrifugo/libcentrifugo/priority"
)

// memoryEngine allows to run Centrifugo without using Redis at all. All data managed inside process
// memory. With this engine you can only run single Centrifugo node. If you need to scale you should
// use Redis engine instead.
type memoryEngine struct {
	app         *application
	presenceHub *memoryPresenceHub
	historyHub  *memoryHistoryHub
}

func newMemoryEngine(app *application) *memoryEngine {
	return &memoryEngine{
		app:         app,
		presenceHub: newMemoryPresenceHub(),
		historyHub:  newMemoryHistoryHub(),
	}
}

func (e *memoryEngine) name() string {
	return "In memory – single node only"
}

func (e *memoryEngine) initialize() error {
	err := e.historyHub.initialize()
	return err
}

func (e *memoryEngine) publish(channel Channel, message []byte) error {
	return e.app.handleMsg(channel, message)
}

func (e *memoryEngine) subscribe(channel Channel) error {
	return nil
}

func (e *memoryEngine) unsubscribe(channel Channel) error {
	return nil
}

func (e *memoryEngine) addPresence(channel Channel, uid ConnID, info ClientInfo) error {
	return e.presenceHub.add(channel, uid, info)
}

func (e *memoryEngine) removePresence(channel Channel, uid ConnID) error {
	return e.presenceHub.remove(channel, uid)
}

func (e *memoryEngine) presence(channel Channel) (map[ConnID]ClientInfo, error) {
	return e.presenceHub.get(channel)
}

func (e *memoryEngine) addHistoryMessage(channel Channel, message Message, size, lifetime int64) error {
	return e.historyHub.add(channel, message, size, lifetime)
}

func (e *memoryEngine) history(channel Channel) ([]Message, error) {
	return e.historyHub.get(channel)
}

type memoryPresenceHub struct {
	sync.Mutex
	presence map[Channel]map[ConnID]ClientInfo
}

func newMemoryPresenceHub() *memoryPresenceHub {
	return &memoryPresenceHub{
		presence: make(map[Channel]map[ConnID]ClientInfo),
	}
}

func (h *memoryPresenceHub) add(channel Channel, uid ConnID, info ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[channel]
	if !ok {
		h.presence[channel] = make(map[ConnID]ClientInfo)
	}
	h.presence[channel][uid] = info
	return nil
}

func (h *memoryPresenceHub) remove(channel Channel, uid ConnID) error {
	h.Lock()
	defer h.Unlock()

	if _, ok := h.presence[channel]; !ok {
		return nil
	}
	if _, ok := h.presence[channel][uid]; !ok {
		return nil
	}

	delete(h.presence[channel], uid)

	// clean up map if needed
	if len(h.presence[channel]) == 0 {
		delete(h.presence, channel)
	}

	return nil
}

func (h *memoryPresenceHub) get(channel Channel) (map[ConnID]ClientInfo, error) {
	h.Lock()
	defer h.Unlock()

	presence, ok := h.presence[channel]
	if !ok {
		// return empty map
		return map[ConnID]ClientInfo{}, nil
	}
	// FIXME: Return copy, since we release the lock
	return presence, nil
}

type historyItem struct {
	messages []Message
	expireAt int64
}

func (i historyItem) isExpired() bool {
	return i.expireAt < time.Now().Unix()
}

type memoryHistoryHub struct {
	sync.Mutex // FIXME: Change to RWLock
	history    map[Channel]historyItem
	queue      priority.Queue
	nextCheck  int64
}

func newMemoryHistoryHub() *memoryHistoryHub {
	return &memoryHistoryHub{
		history:   make(map[Channel]historyItem),
		queue:     priority.MakeQueue(),
		nextCheck: 0,
	}
}

func (h *memoryHistoryHub) initialize() error {
	go h.expire()
	return nil
}

func (h *memoryHistoryHub) expire() {
	for {
		time.Sleep(time.Second)
		h.Lock()
		if h.nextCheck == 0 || h.nextCheck > time.Now().Unix() {
			h.Unlock()
			continue
		}
		for h.queue.Len() > 0 {
			item := heap.Pop(&h.queue).(*priority.Item)
			expireAt := item.Priority
			if expireAt > time.Now().Unix() {
				heap.Push(&h.queue, item)
				break
			}
			channel := Channel(item.Value)
			hItem, ok := h.history[channel]
			if !ok {
				continue
			}
			if hItem.expireAt <= expireAt {
				delete(h.history, channel)
			}
		}
		h.nextCheck = h.nextCheck + 300
		h.Unlock()
	}
}

func (h *memoryHistoryHub) add(channel Channel, message Message, size, lifetime int64) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.history[channel]

	expireAt := time.Now().Unix() + lifetime
	heap.Push(&h.queue, &priority.Item{Value: string(channel), Priority: expireAt})

	if !ok {
		h.history[channel] = historyItem{
			messages: []Message{message},
			expireAt: expireAt,
		}
	} else {
		messages := h.history[channel].messages
		messages = append([]Message{message}, messages...)
		if int64(len(messages)) > size {
			messages = messages[0:size]
		}
		h.history[channel] = historyItem{
			messages: messages,
			expireAt: expireAt,
		}
	}

	if h.nextCheck == 0 || h.nextCheck > expireAt {
		h.nextCheck = expireAt
	}

	return nil
}

func (h *memoryHistoryHub) get(channel Channel) ([]Message, error) {
	h.Lock()
	defer h.Unlock()

	hItem, ok := h.history[channel]
	if !ok {
		// return empty slice
		return []Message{}, nil
	}
	if hItem.isExpired() {
		// return empty slice
		delete(h.history, channel)
		return []Message{}, nil
	}
	return hItem.messages, nil
}
