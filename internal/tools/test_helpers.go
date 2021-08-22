package tools

import (
	"io"
	"sync"

	"github.com/centrifugal/centrifuge"
)

// TestTransport - test transport
type TestTransport struct {
	mu         sync.Mutex
	sink       chan []byte
	closed     bool
	closeCh    chan struct{}
	disconnect *centrifuge.Disconnect
	protoType  centrifuge.ProtocolType
}

// NewTestTransport - builder for TestTransport
func NewTestTransport() *TestTransport {
	return &TestTransport{
		protoType: centrifuge.ProtocolTypeJSON,
		closeCh:   make(chan struct{}),
	}
}

// Write - ...
func (t *TestTransport) Write(message []byte) error {
	return t.WriteMany(message)
}

// WriteMany - ...
func (t *TestTransport) WriteMany(messages ...[]byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return io.EOF
	}
	for _, buf := range messages {
		dataCopy := make([]byte, len(buf))
		copy(dataCopy, buf)
		if t.sink != nil {
			t.sink <- dataCopy
		}
	}
	return nil
}

// Name - ...
func (t *TestTransport) Name() string {
	return "test_transport"
}

// Protocol - ...
func (t *TestTransport) Protocol() centrifuge.ProtocolType {
	return t.protoType
}

// Unidirectional - ...
func (t *TestTransport) Unidirectional() bool {
	return false
}

// DisabledPushFlags - ...
func (t *TestTransport) DisabledPushFlags() uint64 {
	return centrifuge.PushFlagDisconnect
}

// Close - ...
func (t *TestTransport) Close(disconnect *centrifuge.Disconnect) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.disconnect = disconnect
	t.closed = true
	close(t.closeCh)
	return nil
}

// NodeWithMemoryEngineNoHandlers - builder for centrifuge node with memory engine
func NodeWithMemoryEngineNoHandlers() *centrifuge.Node {
	c := centrifuge.DefaultConfig
	n, err := centrifuge.New(c)
	if err != nil {
		panic(err)
	}
	err = n.Run()
	if err != nil {
		panic(err)
	}
	return n
}

// NodeWithMemoryEngine - builder for centrifuge node with memory engine
func NodeWithMemoryEngine() *centrifuge.Node {
	n := NodeWithMemoryEngineNoHandlers()
	n.OnConnect(func(client *centrifuge.Client) {
		client.OnSubscribe(func(_ centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			cb(centrifuge.SubscribeReply{}, nil)
		})
		client.OnPublish(func(_ centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			cb(centrifuge.PublishReply{}, nil)
		})
	})
	return n
}
