package server

import (
	"sync"

	"github.com/centrifugal/centrifugo/lib/internal/queue"
	"github.com/centrifugal/centrifugo/lib/proto"
)

type writerConfig struct {
	MaxQueueSize         int
	QueueInitialCapacity int
}

// writer helps to manage per-connection message queue.
type writer struct {
	mu       sync.Mutex
	config   writerConfig
	writeFn  func([]byte) error
	messages queue.Queue
	closed   bool
}

func newWriter(config writerConfig) *writer {
	w := &writer{
		config:   config,
		messages: queue.New(config.QueueInitialCapacity),
	}
	go w.runWriteRoutine()
	return w
}

func (w *writer) runWriteRoutine() {
	for {
		msg, ok := w.messages.Wait()
		if !ok {
			if w.messages.Closed() {
				return
			}
			continue
		}

		err := w.writeFn(msg)
		if err != nil {
			// Write failed, transport must close itself, here we just return from routine.
			return
		}
	}
}

func (w *writer) write(data []byte) *proto.Disconnect {
	ok := w.messages.Add(data)
	if !ok {
		return nil
	}
	if w.messages.Size() > w.config.MaxQueueSize {
		return &proto.Disconnect{Reason: "slow", Reconnect: true}
	}
	return nil
}

func (w *writer) onWrite(writeFn func([]byte) error) {
	w.writeFn = writeFn
}

func (w *writer) close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.messages.Close()
	w.closed = true
	w.mu.Unlock()
	return nil
}
