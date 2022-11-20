package wt

import (
	"sync"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/marten-seemann/webtransport-go"
)

const transportName = "webtransport"

type webtransportTransport struct {
	mu             sync.RWMutex
	closeCh        chan struct{}
	protoType      centrifuge.ProtocolType
	session        *webtransport.Session
	stream         webtransport.Stream
	pingPongConfig centrifuge.PingPongConfig
	closed         bool
}

func newWebtransportTransport(protoType centrifuge.ProtocolType, session *webtransport.Session, stream webtransport.Stream, pingPongConfig centrifuge.PingPongConfig) *webtransportTransport {
	return &webtransportTransport{
		protoType:      protoType,
		closeCh:        make(chan struct{}),
		session:        session,
		stream:         stream,
		pingPongConfig: pingPongConfig,
	}
}

// Name implementation.
func (t *webtransportTransport) Name() string {
	return transportName
}

// Protocol implementation.
func (t *webtransportTransport) Protocol() centrifuge.ProtocolType {
	return t.protoType
}

// Unidirectional implementation.
func (t *webtransportTransport) Unidirectional() bool {
	return false
}

// DisabledPushFlags ...
func (t *webtransportTransport) DisabledPushFlags() uint64 {
	return 0
}

// ProtocolVersion ...
func (t *webtransportTransport) ProtocolVersion() centrifuge.ProtocolVersion {
	return centrifuge.ProtocolVersion2
}

// Emulation ...
func (t *webtransportTransport) Emulation() bool {
	return false
}

// AppLevelPing ...
func (t *webtransportTransport) AppLevelPing() centrifuge.AppLevelPing {
	return centrifuge.AppLevelPing{
		PingInterval: t.pingPongConfig.PingInterval,
		PongTimeout:  t.pingPongConfig.PongTimeout,
	}
}

const writeTimeout = 1 * time.Second

// Write ...
func (t *webtransportTransport) Write(message []byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		protoType := protocol.TypeJSON
		if t.protoType == centrifuge.ProtocolTypeProtobuf {
			protoType = protocol.TypeProtobuf
		}
		encoder := protocol.GetDataEncoder(protoType)
		defer protocol.PutDataEncoder(protoType, encoder)
		_ = encoder.Encode(message)
		_ = t.stream.SetWriteDeadline(time.Now().Add(writeTimeout))
		_, err := t.stream.Write(encoder.Finish())
		if err != nil {
			return err
		}
		if protoType == protocol.TypeJSON {
			// Need extra new line since WebTransport is stream-based, not frame-based.
			_, err = t.stream.Write([]byte("\n"))
		}
		_ = t.stream.SetWriteDeadline(time.Time{})
		return err
	}
}

// WriteMany ...
func (t *webtransportTransport) WriteMany(messages ...[]byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		protoType := protocol.TypeJSON
		if t.protoType == centrifuge.ProtocolTypeProtobuf {
			protoType = protocol.TypeProtobuf
		}
		encoder := protocol.GetDataEncoder(protoType)
		defer protocol.PutDataEncoder(protoType, encoder)

		for i := range messages {
			err := encoder.Encode(messages[i])
			if err != nil {
				return err
			}
		}

		_ = t.stream.SetWriteDeadline(time.Now().Add(writeTimeout))
		_, err := t.stream.Write(encoder.Finish())
		if err != nil {
			return err
		}
		if protoType == protocol.TypeJSON {
			// Need extra new line since WebTransport is stream-based, not frame-based.
			_, err = t.stream.Write([]byte("\n"))
		}
		_ = t.stream.SetWriteDeadline(time.Time{})
		return err
	}
}

// Close ...
func (t *webtransportTransport) Close(d centrifuge.Disconnect) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.closeCh)
	t.mu.Unlock()

	_ = t.stream.Close()

	// Seems we hit https://github.com/lucas-clemente/quic-go/issues/3291 here.
	// Adding sleep to give client a chance to receive the data. This is mostly
	// important for disconnect advices as sometimes we don't want clients to
	// reconnect. This may actually become obsolete if we will have a way to send
	// WebTransportCloseInfo https://www.w3.org/TR/webtransport/#dictdef-webtransportcloseinfo
	// which is currently not-supported by webtransport-go.
	// TODO: check whether we still need this since we are sending close info now.
	time.Sleep(time.Second)

	return t.session.CloseWithError(webtransport.SessionErrorCode(d.Code), d.Reason)
}
