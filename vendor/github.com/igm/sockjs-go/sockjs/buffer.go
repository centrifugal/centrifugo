package sockjs

import "sync"

// messageBuffer is an unbounded buffer that blocks on
// pop if it's empty until the new element is enqueued.
type messageBuffer struct {
	popCh   chan string
	closeCh chan struct{}
	once    sync.Once // for b.close()
}

func newMessageBuffer() *messageBuffer {
	return &messageBuffer{
		popCh:   make(chan string),
		closeCh: make(chan struct{}),
	}
}

func (b *messageBuffer) push(messages ...string) error {
	for _, message := range messages {
		select {
		case b.popCh <- message:
		case <-b.closeCh:
			return ErrSessionNotOpen
		}
	}

	return nil
}

func (b *messageBuffer) pop() (string, error) {
	select {
	case msg := <-b.popCh:
		return msg, nil
	case <-b.closeCh:
		return "", ErrSessionNotOpen
	}
}

func (b *messageBuffer) close() { b.once.Do(func() { close(b.closeCh) }) }
