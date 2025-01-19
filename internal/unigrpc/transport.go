package unigrpc

import (
	"sync"

	"github.com/centrifugal/centrifugo/v6/internal/unigrpc/unistream"

	"github.com/centrifugal/centrifuge"
)

// grpcTransport wraps a stream.
type grpcTransport struct {
	mu           sync.RWMutex
	stream       unistream.CentrifugoUniStream_ConsumeServer
	closed       bool
	closeCh      chan struct{}
	streamDataCh chan rawFrame
}

func newGRPCTransport(stream unistream.CentrifugoUniStream_ConsumeServer, streamDataCh chan rawFrame) *grpcTransport {
	return &grpcTransport{
		stream:       stream,
		streamDataCh: streamDataCh,
		closeCh:      make(chan struct{}),
	}
}

const transportName = "uni_grpc"

func (t *grpcTransport) Name() string {
	return transportName
}

func (t *grpcTransport) Protocol() centrifuge.ProtocolType {
	return centrifuge.ProtocolTypeProtobuf
}

// ProtocolVersion returns transport protocol version.
func (t *grpcTransport) ProtocolVersion() centrifuge.ProtocolVersion {
	return centrifuge.ProtocolVersion2
}

// Unidirectional returns whether transport is unidirectional.
func (t *grpcTransport) Unidirectional() bool {
	return true
}

// DisabledPushFlags ...
func (t *grpcTransport) DisabledPushFlags() uint64 {
	return 0
}

// PingPongConfig ...
func (t *grpcTransport) PingPongConfig() centrifuge.PingPongConfig {
	return centrifuge.PingPongConfig{
		PingInterval: 0,
	}
}

// Emulation ...
func (t *grpcTransport) Emulation() bool {
	return false
}

func (t *grpcTransport) Write(message []byte) error {
	return t.WriteMany(message)
}

func (t *grpcTransport) WriteMany(messages ...[]byte) error {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil
	}
	t.mu.RUnlock()
	for i := 0; i < len(messages); i++ {
		err := t.stream.SendMsg(rawFrame(messages[i]))
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *grpcTransport) Close(_ centrifuge.Disconnect) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	close(t.closeCh)
	return nil
}
