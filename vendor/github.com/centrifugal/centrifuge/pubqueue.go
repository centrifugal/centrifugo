package centrifuge

import (
	"sync"

	"github.com/centrifugal/centrifuge/internal/prepared"
	"github.com/centrifugal/protocol"
)

type preparedPub struct {
	reply   *prepared.Reply
	pub     *protocol.Publication
	chOpts  *ChannelOptions
	channel string
}

type pubQueue struct {
	mu      sync.RWMutex
	cond    *sync.Cond
	nodes   []preparedPub
	head    int
	tail    int
	cnt     int
	size    int
	closed  bool
	initCap int
}

var initialCapacity = 1

// New ByteQueue returns a new preparedPub queue with initial capacity.
func newPubQueue() *pubQueue {
	sq := &pubQueue{
		initCap: initialCapacity,
		nodes:   make([]preparedPub, initialCapacity),
	}
	sq.cond = sync.NewCond(&sq.mu)
	return sq
}

// Write mutex must be held when calling
func (q *pubQueue) resize(n int) {
	nodes := make([]preparedPub, n)
	if q.head < q.tail {
		copy(nodes, q.nodes[q.head:q.tail])
	} else {
		copy(nodes, q.nodes[q.head:])
		copy(nodes[len(q.nodes)-q.head:], q.nodes[:q.tail])
	}

	q.tail = q.cnt % n
	q.head = 0
	q.nodes = nodes
}

// Add a preparedPub to the back of the queue
// will return false if the queue is closed.
// In that case the preparedPub is dropped.
func (q *pubQueue) Add(i preparedPub) bool {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return false
	}
	if q.cnt == len(q.nodes) {
		// Also tested a grow rate of 1.5, see: http://stackoverflow.com/questions/2269063/buffer-growth-strategy
		// In Go this resulted in a higher memory usage.
		q.resize(q.cnt * 2)
	}
	q.nodes[q.tail] = i
	q.tail = (q.tail + 1) % len(q.nodes)
	q.size += len(i.reply.Data())
	q.cnt++
	q.cond.Signal()
	q.mu.Unlock()
	return true
}

// Close the queue and discard all entries in the queue
// all goroutines in wait() will return
func (q *pubQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cnt = 0
	q.nodes = nil
	q.size = 0
	q.cond.Broadcast()
}

// Closed returns true if the queue has been closed
// The call cannot guarantee that the queue hasn't been
// closed while the function returns, so only "true" has a definite meaning.
func (q *pubQueue) Closed() bool {
	q.mu.RLock()
	c := q.closed
	q.mu.RUnlock()
	return c
}

// Wait for a preparedPub to be added.
// If there is items on the queue the first will
// be returned immediately.
// Will return nil, false if the queue is closed.
// Otherwise the return value of "remove" is returned.
func (q *pubQueue) Wait() (preparedPub, bool) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return preparedPub{}, false
	}
	if q.cnt != 0 {
		q.mu.Unlock()
		return q.Remove()
	}
	q.cond.Wait()
	q.mu.Unlock()
	return q.Remove()
}

// Remove will remove a preparedPub from the queue.
// If false is returned, it either means 1) there were no items on the queue
// or 2) the queue is closed.
func (q *pubQueue) Remove() (preparedPub, bool) {
	q.mu.Lock()
	if q.cnt == 0 {
		q.mu.Unlock()
		return preparedPub{}, false
	}
	i := q.nodes[q.head]
	q.head = (q.head + 1) % len(q.nodes)
	q.cnt--
	q.size -= len(i.reply.Data())

	if n := len(q.nodes) / 2; n >= q.initCap && q.cnt <= n {
		q.resize(n)
	}

	q.mu.Unlock()
	return i, true
}

// Size returns the current size of the queue.
func (q *pubQueue) Size() int {
	q.mu.RLock()
	s := q.size
	q.mu.RUnlock()
	return s
}
