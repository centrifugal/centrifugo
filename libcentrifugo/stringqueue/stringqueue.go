package stringqueue

import (
	"sync"
)

// StringQueue is an unbounded queue of strings.
// The queue is goroutine safe.
// Inspired by http://blog.dubbelboer.com/2015/04/25/go-faster-queue.html (MIT)
type StringQueue interface {
	// Add a string to the back of the queue
	// will return false if the queue is closed.
	// In that case the string is dropped.
	Add(i string) bool

	// Remove will remove a string from the queue.
	// If false is returned, it either means 1) there were no items on the queue
	// or 2) the queue is closed.
	Remove() (string, bool)

	// Close the queue and discard all entried in the queue
	// all goroutines in wait() will return
	Close()

	// Closed returns true if the queue has been closed
	// The call cannot guarantee that the queue hasn't been
	// closed while the function returns, so only "true" has a definite meaning.
	Closed() bool

	// Wait for a string to be added or queue to be closed.
	// If there is items on the queue the first will
	// be returned immediately.
	// Will return "", false if the queue is closed.
	// Otherwise the return value of "remove" is returned.
	Wait() (string, bool)

	// Cap returns the capacity (without allocations)
	Cap() int

	// Len returns the current length of the queue.
	Len() int
}

type stringQueue struct {
	mu       sync.RWMutex
	cond     *sync.Cond
	nodes    []string
	head     int
	tail     int
	cnt      int
	isClosed bool
}

const (
	initialCapacity = 2
)

// NewStringQueue returns a new string queue with initial capacity.
func New() StringQueue {
	sq := &stringQueue{
		nodes: make([]string, initialCapacity),
	}
	sq.cond = sync.NewCond(&sq.mu)
	return sq
}

// Write mutex must be held when calling
func (q *stringQueue) resize(n int) {
	nodes := make([]string, n)
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

// Add a string to the back of the queue
// will return false if the queue is closed.
// In that case the string is dropped.
func (q *stringQueue) Add(i string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.isClosed {
		return false
	}
	if q.cnt == len(q.nodes) {
		// Also tested a grow rate of 1.5, see: http://stackoverflow.com/questions/2269063/buffer-growth-strategy
		// In Go this resulted in a higher memory usage.
		q.resize(q.cnt * 2)
	}
	q.nodes[q.tail] = i
	q.tail = (q.tail + 1) % len(q.nodes)
	q.cnt++
	q.cond.Signal()
	return true
}

// Close the queue and discard all entried in the queue
// all goroutines in wait() will return
func (q *stringQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.isClosed = true
	q.cnt = 0
	q.nodes = nil
	q.cond.Broadcast()
}

// Closed returns true if the queue has been closed
// The call cannot guarantee that the queue hasn't been
// closed while the function returns, so only "true" has a definite meaning.
func (q *stringQueue) Closed() bool {
	q.mu.RLock()
	c := q.isClosed
	q.mu.RUnlock()
	return c
}

// Wait for a string to be added.
// If there is items on the queue the first will
// be returned immediately.
// Will return "", false if the queue is closed.
// Otherwise the return value of "remove" is returned.
func (q *stringQueue) Wait() (string, bool) {
	q.mu.Lock()
	if q.isClosed {
		q.mu.Unlock()
		return "", false
	}
	if q.cnt != 0 {
		q.mu.Unlock()
		return q.Remove()
	}
	q.cond.Wait()
	q.mu.Unlock()
	return q.Remove()
}

// Remove will remove a string from the queue.
// If false is returned, it either means 1) there were no items on the queue
// or 2) the queue is closed.
func (q *stringQueue) Remove() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cnt == 0 {
		return "", false
	}
	i := q.nodes[q.head]
	q.head = (q.head + 1) % len(q.nodes)
	q.cnt--

	if n := len(q.nodes) / 2; n >= 2 && q.cnt <= n {
		q.resize(n)
	}

	return i, true
}

// Return the capacity (without allocations)
func (q *stringQueue) Cap() int {
	q.mu.RLock()
	c := cap(q.nodes)
	q.mu.RUnlock()
	return c
}

// Return the current length of the queue.
func (q *stringQueue) Len() int {
	q.mu.RLock()
	l := q.cnt
	q.mu.RUnlock()
	return l
}
