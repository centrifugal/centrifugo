package clientservice

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/centrifugal/centrifugo/lib/proto"
	"google.golang.org/grpc/metadata"
)

// grpcTransport represents wrapper over stream to work with it
// from outside in abstract way.
type grpcTransport struct {
	mu      sync.Mutex
	closed  bool
	stream  proto.Centrifugo_CommunicateServer
	replies chan *proto.Reply
}

func newGRPCTransport(stream proto.Centrifugo_CommunicateServer, replies chan *proto.Reply) *grpcTransport {
	return &grpcTransport{
		stream:  stream,
		replies: replies,
	}
}

func (t *grpcTransport) Name() string {
	return "grpc"
}

func (t *grpcTransport) Encoding() proto.Encoding {
	return proto.EncodingProtobuf
}

func (t *grpcTransport) Send(reply *proto.PreparedReply) error {
	select {
	case t.replies <- reply.Reply:
	default:
		return fmt.Errorf("error sending to transport: buffer channel is full")
	}
	return nil
}

func (t *grpcTransport) Close(disconnect *proto.Disconnect) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	disconnectJSON, err := json.Marshal(disconnect)
	if err != nil {
		return err
	}
	t.stream.SetTrailer(metadata.Pairs("disconnect", string(disconnectJSON)))
	close(t.replies)
	return nil
}
