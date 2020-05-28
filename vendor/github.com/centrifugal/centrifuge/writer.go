package centrifuge

import (
	"sync"

	"github.com/centrifugal/centrifuge/internal/queue"
)

type writerConfig struct {
	WriteManyFn        func(...[]byte) error
	WriteFn            func([]byte) error
	MaxQueueSize       int
	MaxMessagesInFrame int
}

// writer helps to manage per-connection message queue.
type writer struct {
	mu       sync.Mutex
	config   writerConfig
	messages queue.Queue
	closed   bool
}

func newWriter(config writerConfig) *writer {
	w := &writer{
		config:   config,
		messages: queue.New(),
	}
	go w.runWriteRoutine()
	return w
}

const (
	defaultMaxMessagesInFrame = 4
)

func (w *writer) runWriteRoutine() {
	maxMessagesInFrame := w.config.MaxMessagesInFrame
	if maxMessagesInFrame == 0 {
		maxMessagesInFrame = defaultMaxMessagesInFrame
	}

	for {
		// Wait for message from queue.
		msg, ok := w.messages.Wait()
		if !ok {
			if w.messages.Closed() {
				return
			}
			continue
		}

		var writeErr error

		messageCount := w.messages.Len()
		if maxMessagesInFrame > 1 && messageCount > 0 {
			// There are several more messages left in queue, try to send them in single frame,
			// but no more than maxMessagesInFrame.

			// Limit message count to get from queue with (maxMessagesInFrame - 1)
			// (as we already have one message received from queue above).
			messagesCap := messageCount + 1
			if messagesCap > maxMessagesInFrame {
				messagesCap = maxMessagesInFrame
			}

			msgs := make([][]byte, 0, messagesCap)
			msgs = append(msgs, msg)

			for messageCount > 0 {
				messageCount--
				if len(msgs) >= maxMessagesInFrame {
					break
				}
				m, ok := w.messages.Remove()
				if ok {
					msgs = append(msgs, m)
				} else {
					if w.messages.Closed() {
						return
					}
					break
				}
			}
			if len(msgs) > 0 {
				w.mu.Lock()
				if len(msgs) == 1 {
					writeErr = w.config.WriteFn(msgs[0])
				} else {
					writeErr = w.config.WriteManyFn(msgs...)
				}
				w.mu.Unlock()
			}
		} else {
			// Write single message without allocating new [][]byte slice.
			w.mu.Lock()
			writeErr = w.config.WriteFn(msg)
			w.mu.Unlock()
		}
		if writeErr != nil {
			// Write failed, transport must close itself, here we just return from routine.
			return
		}
	}
}

func (w *writer) enqueue(data []byte) *Disconnect {
	ok := w.messages.Add(data)
	if !ok {
		return DisconnectNormal
	}
	if w.config.MaxQueueSize > 0 && w.messages.Size() > w.config.MaxQueueSize {
		return DisconnectSlow
	}
	return nil
}

func (w *writer) close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()

	remaining := w.messages.CloseRemaining()
	if len(remaining) > 0 {
		w.mu.Lock()
		_ = w.config.WriteManyFn(remaining...)
		w.mu.Unlock()
	}

	return nil
}
