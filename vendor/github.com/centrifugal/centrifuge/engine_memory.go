package centrifuge

import (
	"container/heap"
	"strconv"
	"sync"
	"time"

	"github.com/centrifugal/centrifuge/internal/priority"
)

// MemoryEngine is builtin default engine which allows to run Centrifuge-based
// server without any external brokers/storages. All data managed inside process
// memory.
//
// With this engine you can only run single Centrifuge node. If you need to scale
// you should use another engine implementation instead – for example Redis engine.
//
// Running single node can be sufficient for many use cases especially when you
// need maximum performance and do not too many online clients. Consider configuring
// your load balancer to have one backup Centrifuge node for HA in this case.
type MemoryEngine struct {
	node         *Node
	presenceHub  *presenceHub
	historyHub   *historyHub
	eventHandler BrokerEventHandler
}

var _ Engine = (*MemoryEngine)(nil)

// MemoryEngineConfig is a memory engine config.
type MemoryEngineConfig struct{}

// NewMemoryEngine initializes Memory Engine.
func NewMemoryEngine(n *Node, _ MemoryEngineConfig) (*MemoryEngine, error) {
	e := &MemoryEngine{
		node:        n,
		presenceHub: newPresenceHub(),
		historyHub:  newHistoryHub(),
	}
	e.historyHub.initialize()
	return e, nil
}

// Run runs memory engine - we do not have any logic here as Memory Engine ready to work
// just after initialization.
func (e *MemoryEngine) Run(h BrokerEventHandler) error {
	e.eventHandler = h
	return nil
}

// Publish adds message into history hub and calls node ClientMsg method to handle message.
// We don't have any PUB/SUB here as Memory Engine is single node only.
func (e *MemoryEngine) Publish(ch string, pub *Publication, _ *ChannelOptions) error {
	return e.eventHandler.HandlePublication(ch, pub)
}

// PublishJoin - see engine interface description.
func (e *MemoryEngine) PublishJoin(ch string, join *Join, _ *ChannelOptions) error {
	return e.eventHandler.HandleJoin(ch, join)
}

// PublishLeave - see engine interface description.
func (e *MemoryEngine) PublishLeave(ch string, leave *Leave, _ *ChannelOptions) error {
	return e.eventHandler.HandleLeave(ch, leave)
}

// PublishControl - see Engine interface description.
func (e *MemoryEngine) PublishControl(data []byte) error {
	return e.eventHandler.HandleControl(data)
}

// Subscribe is noop here.
func (e *MemoryEngine) Subscribe(_ string) error {
	return nil
}

// Unsubscribe node from channel.
func (e *MemoryEngine) Unsubscribe(_ string) error {
	return nil
}

// AddPresence - see engine interface description.
func (e *MemoryEngine) AddPresence(ch string, uid string, info *ClientInfo, _ time.Duration) error {
	return e.presenceHub.add(ch, uid, info)
}

// RemovePresence - see engine interface description.
func (e *MemoryEngine) RemovePresence(ch string, uid string) error {
	return e.presenceHub.remove(ch, uid)
}

// Presence - see engine interface description.
func (e *MemoryEngine) Presence(ch string) (map[string]*ClientInfo, error) {
	return e.presenceHub.get(ch)
}

// PresenceStats - see engine interface description.
func (e *MemoryEngine) PresenceStats(ch string) (PresenceStats, error) {
	return e.presenceHub.getStats(ch)
}

// History - see engine interface description.
func (e *MemoryEngine) History(ch string, filter HistoryFilter) ([]*Publication, RecoveryPosition, error) {
	return e.historyHub.get(ch, filter)
}

// AddHistory - see engine interface description.
func (e *MemoryEngine) AddHistory(ch string, pub *Publication, opts *ChannelOptions) (*Publication, error) {
	return e.historyHub.add(ch, pub, opts)
}

// RemoveHistory - see engine interface description.
func (e *MemoryEngine) RemoveHistory(ch string) error {
	return e.historyHub.remove(ch)
}

// Channels - see engine interface description.
func (e *MemoryEngine) Channels() ([]string, error) {
	return e.node.Hub().Channels(), nil
}

type presenceHub struct {
	sync.RWMutex
	presence map[string]map[string]*ClientInfo
}

func newPresenceHub() *presenceHub {
	return &presenceHub{
		presence: make(map[string]map[string]*ClientInfo),
	}
}

func (h *presenceHub) add(ch string, uid string, info *ClientInfo) error {
	h.Lock()
	defer h.Unlock()

	_, ok := h.presence[ch]
	if !ok {
		h.presence[ch] = make(map[string]*ClientInfo)
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

func (h *presenceHub) get(ch string) (map[string]*ClientInfo, error) {
	h.RLock()
	defer h.RUnlock()

	presence, ok := h.presence[ch]
	if !ok {
		// return empty map
		return nil, nil
	}

	data := make(map[string]*ClientInfo, len(presence))
	for k, v := range presence {
		data[k] = v
	}
	return data, nil
}

func (h *presenceHub) getStats(ch string) (PresenceStats, error) {
	h.RLock()
	defer h.RUnlock()

	presence, ok := h.presence[ch]
	if !ok {
		// return empty map
		return PresenceStats{}, nil
	}

	numClients := len(presence)
	numUsers := 0
	uniqueUsers := map[string]struct{}{}

	for _, info := range presence {
		userID := info.User
		if _, ok := uniqueUsers[userID]; !ok {
			uniqueUsers[userID] = struct{}{}
			numUsers++
		}
	}

	return PresenceStats{
		NumClients: numClients,
		NumUsers:   numUsers,
	}, nil
}

type historyItem struct {
	messages []*Publication
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

	epoch       string
	sequencesMu sync.RWMutex
	sequences   map[string]uint64
}

func newHistoryHub() *historyHub {
	return &historyHub{
		history:   make(map[string]historyItem),
		queue:     priority.MakeQueue(),
		nextCheck: 0,
		epoch:     strconv.FormatInt(time.Now().Unix(), 10),
		sequences: make(map[string]uint64),
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

func (h *historyHub) next(ch string) uint64 {
	var val uint64
	h.sequencesMu.Lock()
	top, ok := h.sequences[ch]
	if !ok {
		val = 1
		h.sequences[ch] = val
	} else {
		top++
		h.sequences[ch] = top
		val = top
	}
	h.sequencesMu.Unlock()
	return val
}

func (h *historyHub) getSequence(ch string) (uint32, uint32, string) {
	h.sequencesMu.Lock()
	defer h.sequencesMu.Unlock()
	val, ok := h.sequences[ch]
	if !ok {
		var top uint64
		h.sequences[ch] = top
		return 0, 0, h.epoch
	}
	seq, gen := unpackUint64(val)
	return seq, gen, h.epoch
}

func (h *historyHub) add(ch string, pub *Publication, opts *ChannelOptions) (*Publication, error) {
	h.Lock()
	defer h.Unlock()

	index := h.next(ch)
	pub.Seq, pub.Gen = unpackUint64(index)

	_, ok := h.history[ch]

	expireAt := time.Now().Unix() + int64(opts.HistoryLifetime)
	heap.Push(&h.queue, &priority.Item{Value: ch, Priority: expireAt})
	if !ok {
		h.history[ch] = historyItem{
			messages: []*Publication{pub},
			expireAt: expireAt,
		}
	} else {
		messages := h.history[ch].messages
		messages = append([]*Publication{pub}, messages...)
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

	return pub, nil
}

func (h *historyHub) get(ch string, filter HistoryFilter) ([]*Publication, RecoveryPosition, error) {
	h.RLock()
	defer h.RUnlock()
	return h.getUnsafe(ch, filter)
}

func (h *historyHub) getPublications(ch string) []*Publication {
	hItem, ok := h.history[ch]
	if !ok {
		return []*Publication{}
	}
	if hItem.isExpired() {
		delete(h.history, ch)
		return []*Publication{}
	}
	pubs := hItem.messages
	pubsCopy := make([]*Publication, len(pubs))
	copy(pubsCopy, pubs)

	for i := len(pubsCopy)/2 - 1; i >= 0; i-- {
		opp := len(pubsCopy) - 1 - i
		pubsCopy[i], pubsCopy[opp] = pubsCopy[opp], pubsCopy[i]
	}
	return pubsCopy
}

func (h *historyHub) getUnsafe(ch string, filter HistoryFilter) ([]*Publication, RecoveryPosition, error) {
	latestSeq, latestGen, latestEpoch := h.getSequence(ch)
	latestPosition := RecoveryPosition{Seq: latestSeq, Gen: latestGen, Epoch: latestEpoch}

	if filter.Since == nil {
		if filter.Limit == 0 {
			return nil, latestPosition, nil
		}
		allPubs := h.getPublications(ch)
		if filter.Limit == -1 || filter.Limit >= len(allPubs) {
			return allPubs, latestPosition, nil
		}
		return allPubs[:filter.Limit], latestPosition, nil
	}

	allPubs := h.getPublications(ch)

	since := filter.Since

	if latestSeq == since.Seq && since.Gen == latestGen && since.Epoch == latestEpoch {
		return nil, latestPosition, nil
	}

	nextSeq, nextGen := nextSeqGen(since.Seq, since.Gen)

	position := -1

	for i := 0; i < len(allPubs); i++ {
		pub := allPubs[i]
		if pub.Seq == since.Seq && pub.Gen == since.Gen {
			position = i + 1
			break
		}
		if pub.Seq == nextSeq && pub.Gen == nextGen {
			position = i
			break
		}
	}

	if position > -1 {
		pubs := allPubs[position:]
		if filter.Limit >= 0 {
			return pubs[:filter.Limit], latestPosition, nil
		}
		return pubs, latestPosition, nil
	}

	if filter.Limit >= 0 {
		return allPubs[:filter.Limit], latestPosition, nil
	}
	return allPubs, latestPosition, nil
}

func (h *historyHub) remove(ch string) error {
	h.RLock()
	defer h.RUnlock()

	_, ok := h.history[ch]
	if ok {
		delete(h.history, ch)
	}
	return nil
}
