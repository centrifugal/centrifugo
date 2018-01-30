package grpcclient

import (
	"fmt"
	"io"

	"github.com/centrifugal/centrifugo/lib/client"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
)

// Config for GRPC API server.
type Config struct {
	// APIKey ...
	APIKey string
	// APIInsecure ...
	APIInsecure bool
}

// Server can answer on GRPC API requests.
type Server struct {
	config Config
	node   *node.Node
}

// New creates new server.
func New(n *node.Node, c Config) *Server {
	return &Server{
		config: c,
		node:   n,
	}
}

func (s *Server) Communicate(stream proto.Centrifugo_CommunicateServer) error {

	replies := make(chan *proto.Reply, 64)
	transport := NewGRPCTransport(stream, replies)

	c := client.New(stream.Context(), s.node, transport, client.Config{})
	defer c.Close(proto.DisconnectNormal)

	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
			fmt.Printf("%#v\n", in)
		}
	}()

	for reply := range replies {
		if err := stream.Send(reply); err != nil {
			return err
		}
	}

	return nil
}

type GRPCTransport struct {
	stream  proto.Centrifugo_CommunicateServer
	replies chan *proto.Reply
}

func NewGRPCTransport(stream proto.Centrifugo_CommunicateServer, replies chan *proto.Reply) *GRPCTransport {
	return &GRPCTransport{
		stream:  stream,
		replies: replies,
	}
}

func (t *GRPCTransport) Name() string {
	return "grpc"
}

func (t *GRPCTransport) Send(reply *proto.PreparedReply) error {
	select {
	case t.replies <- reply.Reply:
	default:
		return fmt.Errorf("error sending to transportL buffer full")
	}
	return nil
}

func (t *GRPCTransport) Close(disconnect *proto.Disconnect) error {
	close(t.replies)
	return nil
}
