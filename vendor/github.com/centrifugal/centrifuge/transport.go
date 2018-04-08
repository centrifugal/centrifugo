package centrifuge

import (
	"sync"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/centrifugal/centrifuge/internal/queue"
)

// preparedReply is structure for encoding reply only once.
type preparedReply struct {
	Enc   proto.Encoding
	Reply *proto.Reply
	data  []byte
	once  sync.Once
}

// newPreparedReply initializes PreparedReply.
func newPreparedReply(reply *proto.Reply, enc proto.Encoding) *preparedReply {
	return &preparedReply{
		Reply: reply,
		Enc:   enc,
	}
}

// Data returns data associated with reply which is only calculated once.
func (r *preparedReply) Data() []byte {
	r.once.Do(func() {
		encoder := proto.GetReplyEncoder(r.Enc)
		encoder.Encode(r.Reply)
		data := encoder.Finish()
		proto.PutReplyEncoder(r.Enc, encoder)
		r.data = data
	})
	return r.data
}

// Transport abstracts a connection transport between server and client.
type Transport interface {
	// Name returns a name of transport used for client connection.
	Name() string
	// Encoding returns transport encoding used.
	Encoding() Encoding
}

type transport interface {
	Transport
	// Send sends data to session.
	Send(*preparedReply) error
	// Close closes transport.
	Close(*Disconnect) error
}

type writerConfig struct {
	MaxQueueSize       int
	MaxMessagesInFrame int
}

// writer helps to manage per-connection message queue.
type writer struct {
	mu       sync.Mutex
	config   writerConfig
	writeFn  func(...[]byte) error
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
				msg, ok := w.messages.Remove()
				if ok {
					msgs = append(msgs, msg)
				} else {
					if w.messages.Closed() {
						return
					}
					break
				}
			}
			if len(msgs) > 0 {
				writeErr = w.writeFn(msgs...)
			}
		} else {
			// Write single message without allocating new [][]byte slice.
			writeErr = w.writeFn(msg)
		}
		if writeErr != nil {
			// Write failed, transport must close itself, here we just return from routine.
			return
		}
	}
}

func (w *writer) write(data []byte) *Disconnect {
	ok := w.messages.Add(data)
	if !ok {
		return nil
	}
	if w.config.MaxQueueSize > 0 && w.messages.Size() > w.config.MaxQueueSize {
		return &Disconnect{Reason: "slow", Reconnect: true}
	}
	return nil
}

func (w *writer) onWrite(writeFn func(...[]byte) error) {
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
