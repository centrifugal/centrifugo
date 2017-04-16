package queue

import (
	"sync"
)

type Item interface {
	Len() int
}

// Queue is an unbounded queue of Item.
// The queue is goroutine safe.
// Inspired by http://blog.dubbelboer.com/2015/04/25/go-faster-queue.html (MIT)
type Queue interface {
	// Add an Item to the back of the queue
	// will return false if the queue is closed.
	// In that case the Item is dropped.
	Add(i Item) bool

	// Remove will remove a Item from the queue.
	// If false is returned, it either means 1) there were no items on the queue
	// or 2) the queue is closed.
	Remove() (Item, bool)

	// Close the queue and discard all entried in the queue
	// all goroutines in wait() will return
	Close()

	// CloseRemaining will close the queue and return all entried in the queue.
	// All goroutines in wait() will return
	CloseRemaining() []Item

	// Closed returns true if the queue has been closed
	// The call cannot guarantee that the queue hasn't been
	// closed while the function returns, so only "true" has a definite meaning.
	Closed() bool

	// Wait for a Item to be added or queue to be closed.
	// If there is items on the queue the first will
	// be returned immediately.
	// Will return "", false if the queue is closed.
	// Otherwise the return value of "remove" is returned.
	Wait() (Item, bool)

	// Cap returns the capacity (without allocations).
	Cap() int

	// Len returns the current length of the queue.
	Len() int

	// Size returns the current size of the queue in bytes.
	Size() int
}

type byteQueue struct {
	mu      sync.RWMutex
	cond    *sync.Cond
	nodes   []Item
	head    int
	tail    int
	cnt     int
	size    int
	closed  bool
	initCap int
}

// New ByteQueue returns a new Item queue with initial capacity.
func New(initialCapacity int) Queue {
	sq := &byteQueue{
		initCap: initialCapacity,
		nodes:   make([]Item, initialCapacity),
	}
	sq.cond = sync.NewCond(&sq.mu)
	return sq
}

// Write mutex must be held when calling
func (q *byteQueue) resize(n int) {
	nodes := make([]Item, n)
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

// Add a Item to the back of the queue
// will return false if the queue is closed.
// In that case the Item is dropped.
func (q *byteQueue) Add(i Item) bool {
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
	q.size += i.Len()
	q.cnt++
	q.cond.Signal()
	q.mu.Unlock()
	return true
}

// Close the queue and discard all entried in the queue
// all goroutines in wait() will return
func (q *byteQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cnt = 0
	q.nodes = nil
	q.size = 0
	q.cond.Broadcast()
}

// CloseRemaining will close the queue and return all entried in the queue.
// All goroutines in wait() will return.
func (q *byteQueue) CloseRemaining() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return []Item{}
	}
	rem := make([]Item, 0, q.cnt)
	for q.cnt > 0 {
		i := q.nodes[q.head]
		q.head = (q.head + 1) % len(q.nodes)
		q.cnt--
		rem = append(rem, i)
	}
	q.closed = true
	q.cnt = 0
	q.nodes = nil
	q.size = 0
	q.cond.Broadcast()
	return rem
}

// Closed returns true if the queue has been closed
// The call cannot guarantee that the queue hasn't been
// closed while the function returns, so only "true" has a definite meaning.
func (q *byteQueue) Closed() bool {
	q.mu.RLock()
	c := q.closed
	q.mu.RUnlock()
	return c
}

// Wait for a Item to be added.
// If there is items on the queue the first will
// be returned immediately.
// Will return nil, false if the queue is closed.
// Otherwise the return value of "remove" is returned.
func (q *byteQueue) Wait() (Item, bool) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil, false
	}
	if q.cnt != 0 {
		q.mu.Unlock()
		return q.Remove()
	}
	q.cond.Wait()
	q.mu.Unlock()
	return q.Remove()
}

// Remove will remove a Item from the queue.
// If false is returned, it either means 1) there were no items on the queue
// or 2) the queue is closed.
func (q *byteQueue) Remove() (Item, bool) {
	q.mu.Lock()
	if q.cnt == 0 {
		q.mu.Unlock()
		return nil, false
	}
	i := q.nodes[q.head]
	q.head = (q.head + 1) % len(q.nodes)
	q.cnt--
	q.size -= i.Len()

	if n := len(q.nodes) / 2; n >= q.initCap && q.cnt <= n {
		q.resize(n)
	}

	q.mu.Unlock()
	return i, true
}

// Return the capacity (without allocations)
func (q *byteQueue) Cap() int {
	q.mu.RLock()
	c := cap(q.nodes)
	q.mu.RUnlock()
	return c
}

// Return the current length of the queue.
func (q *byteQueue) Len() int {
	q.mu.RLock()
	l := q.cnt
	q.mu.RUnlock()
	return l
}

// Return the current size of the queue.
func (q *byteQueue) Size() int {
	q.mu.RLock()
	s := q.size
	q.mu.RUnlock()
	return s
}
