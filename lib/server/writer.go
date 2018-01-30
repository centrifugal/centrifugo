package server

import (
	"sync"

	"github.com/centrifugal/centrifugo/lib/internal/queue"
	"github.com/centrifugal/centrifugo/lib/metrics"
	"github.com/centrifugal/centrifugo/lib/proto"
)

type writerConfig struct {
	MaxQueueSize         int
	QueueInitialCapacity int
}

// writer must manage per-connection queue.
type writer struct {
	mu       sync.Mutex
	config   writerConfig
	fn       func([]byte) error
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

		err := w.fn(msg)
		if err != nil {
			// Write failed, transport must be closed, here we just return from func.
			return
		}
		// TODO: move to transport.
		// metrics.DefaultRegistry.Counters.Inc("client_num_msg_sent")
		// metrics.DefaultRegistry.Counters.Add("client_bytes_out", int64(len(msg)))
	}
}

func (w *writer) write(data []byte) *proto.Disconnect {
	ok := w.messages.Add(data)
	if !ok {
		return nil
	}
	metrics.DefaultRegistry.Counters.Inc("client_num_msg_queued")
	if w.messages.Size() > w.config.MaxQueueSize {
		return &proto.Disconnect{Reason: "slow", Reconnect: true}
	}
	return nil
}

func (w *writer) onWrite(fn func([]byte) error) {
	w.fn = fn
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
