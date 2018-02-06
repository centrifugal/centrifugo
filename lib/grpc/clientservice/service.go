package clientservice

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/centrifugal/centrifugo/lib/client"
	"github.com/centrifugal/centrifugo/lib/logger"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"

	"google.golang.org/grpc/metadata"
)

// Config for GRPC client Service.
type Config struct{}

// Service can work with client GRPC connections.
type Service struct {
	config Config
	node   *node.Node
}

// New creates new Service.
func New(n *node.Node, c Config) *Service {
	return &Service{
		config: c,
		node:   n,
	}
}

const replyBufferSize = 64

// Communicate is a bidirectional stream reading Command and
// sending Reply to client.
func (s *Service) Communicate(stream proto.Centrifugo_CommunicateServer) error {

	replies := make(chan *proto.Reply, replyBufferSize)
	transport := newGRPCTransport(stream, replies)

	c := client.New(stream.Context(), s.node, transport, client.Config{})
	defer c.Close(proto.DisconnectNormal)

	go func() {
		for {
			cmd, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
			rep, disconnect := c.Handle(cmd)
			if disconnect != nil {
				logger.ERROR.Printf("disconnect after handling command %v: %v", cmd, disconnect)
				transport.Close(disconnect)
				return
			}
			err = transport.Send(proto.NewPreparedReply(rep, proto.EncodingProtobuf))
			if err != nil {
				transport.Close(&proto.Disconnect{Reason: "error sending message", Reconnect: true})
				transport.Close(disconnect)
				return
			}
		}
	}()

	for reply := range replies {
		if err := stream.Send(reply); err != nil {
			return err
		}
	}

	return nil
}

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
