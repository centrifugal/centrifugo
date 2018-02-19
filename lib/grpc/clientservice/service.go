package clientservice

import (
	"fmt"
	"io"
	"time"

	"github.com/centrifugal/centrifugo/lib/client"
	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"
	"github.com/centrifugal/centrifugo/lib/proto"
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

	s.node.Logger().Log(logging.NewEntry(logging.DEBUG, "GRPC connection established", map[string]interface{}{"client": c.ID()}))
	defer func(started time.Time) {
		s.node.Logger().Log(logging.NewEntry(logging.DEBUG, "GRPC connection completed", map[string]interface{}{"client": c.ID(), "time": time.Since(started)}))
	}(time.Now())

	go func() {
		for {
			cmd, err := stream.Recv()
			if err == io.EOF {
				c.Close(proto.DisconnectNormal)
				return
			}
			if err != nil {
				c.Close(proto.DisconnectNormal)
				return
			}
			if cmd.ID == 0 {
				s.node.Logger().Log(logging.NewEntry(logging.INFO, "command ID required", map[string]interface{}{"client": c.ID(), "user": c.UserID()}))
				c.Close(proto.DisconnectBadRequest)
				return
			}
			rep, disconnect := c.Handle(cmd)
			if disconnect != nil {
				s.node.Logger().Log(logging.NewEntry(logging.INFO, "disconnect after handling command", map[string]interface{}{"command": fmt.Sprintf("%v", cmd), "client": c.ID(), "user": c.UserID(), "reason": disconnect.Reason}))
				c.Close(disconnect)
				return
			}
			if rep != nil {
				err = transport.Send(proto.NewPreparedReply(rep, proto.EncodingProtobuf))
				if err != nil {
					c.Close(&proto.Disconnect{Reason: "error sending message", Reconnect: true})
					return
				}
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
