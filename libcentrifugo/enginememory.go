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

func (e *memoryEngine) publish(ch ChannelID, message []byte) error {
	return e.app.handleMsg(ch, message)
}

func (e *memoryEngine) subscribe(ch ChannelID) error {
	return nil
}

func (e *memoryEngine) unsubscribe(ch ChannelID) error {
	return nil
}

func (e *memoryEngine) addPresence(ch ChannelID, uid ConnID, info ClientInfo) error {
	return e.presenceHub.add(ch, uid, info)
}

func (e *memoryEngine) removePresence(ch ChannelID, uid ConnID) error {
	return e.presenceHub.remove(ch, uid)
}

func (e *memoryEngine) presence(ch ChannelID) (map[ConnID]ClientInfo, error) {
	return e.presenceHub.get(ch)
}

func (e *memoryEngine) addHistoryMessage(ch ChannelID, message Message, size, lifetime int64) error {
	return e.historyHub.add(ch, message, size, lifetime)
}

func (e *memoryEngine) history(ch ChannelID) ([]Message, error) {
	return e.historyHub.get(ch)
}

type memoryPresenceHub struct {
	sync.Mutex
	presence map[ChannelID]map[ConnID]ClientInfo
}

func newMemoryPresenceHub() *memoryPresenceHub {
	return &memoryPresenceHub{
		presence: make(map[ChannelID]map[ConnID]ClientInfo),
	}
}

func (h *memoryPresenceHub) add(ch ChannelID, uid ConnID, info ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[ch]
	if !ok {
		h.presence[ch] = make(map[ConnID]ClientInfo)
	}
	h.presence[ch][uid] = info
	return nil
}

func (h *memoryPresenceHub) remove(ch ChannelID, uid ConnID) error {
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

func (h *memoryPresenceHub) get(ch ChannelID) (map[ConnID]ClientInfo, error) {
	h.Lock()
	defer h.Unlock()

	presence, ok := h.presence[ch]
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
	history    map[ChannelID]historyItem
	queue      priority.Queue
	nextCheck  int64
}

func newMemoryHistoryHub() *memoryHistoryHub {
	return &memoryHistoryHub{
		history:   make(map[ChannelID]historyItem),
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
			channel := ChannelID(item.Value)
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

func (h *memoryHistoryHub) add(ch ChannelID, message Message, size, lifetime int64) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.history[ch]

	expireAt := time.Now().Unix() + lifetime
	heap.Push(&h.queue, &priority.Item{Value: string(ch), Priority: expireAt})

	if !ok {
		h.history[ch] = historyItem{
			messages: []Message{message},
			expireAt: expireAt,
		}
	} else {
		messages := h.history[ch].messages
		messages = append([]Message{message}, messages...)
		if int64(len(messages)) > size {
			messages = messages[0:size]
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

func (h *memoryHistoryHub) get(ch ChannelID) ([]Message, error) {
	h.Lock()
	defer h.Unlock()

	hItem, ok := h.history[ch]
	if !ok {
		// return empty slice
		return []Message{}, nil
	}
	if hItem.isExpired() {
		// return empty slice
		delete(h.history, ch)
		return []Message{}, nil
	}
	return hItem.messages, nil
}
