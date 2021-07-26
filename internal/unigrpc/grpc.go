package unigrpc

import (
	"time"

	"github.com/centrifugal/centrifugo/v3/internal/unigrpc/unistream"

	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
)

// RegisterService ...
func RegisterService(server *grpc.Server, service unistream.CentrifugoUniStreamServer) error {
	unistream.RegisterCentrifugoUniStreamServer(server, service)
	return nil
}

// Service can work with client GRPC connections.
type Service struct {
	unistream.UnimplementedCentrifugoUniStreamServer
	config Config
	node   *centrifuge.Node
}

// newGRPCClientService creates new Service.
func NewService(n *centrifuge.Node, c Config) *Service {
	return &Service{
		config: c,
		node:   n,
	}
}

// Consume is a unidirectional server->client stream with real-time data.
func (s *Service) Consume(req *unistream.ConnectRequest, stream unistream.CentrifugoUniStream_ConsumeServer) error {
	streamDataCh := make(chan rawFrame)
	transport := newGRPCTransport(stream, streamDataCh)

	connectRequest := centrifuge.ConnectRequest{
		Token:   req.Token,
		Data:    req.Data,
		Name:    req.Name,
		Version: req.Version,
	}
	if req.Subs != nil {
		subs := make(map[string]centrifuge.SubscribeRequest)
		for k, v := range connectRequest.Subs {
			subs[k] = centrifuge.SubscribeRequest{
				Recover: v.Recover,
				Offset:  v.Offset,
				Epoch:   v.Epoch,
			}
		}
	}

	c, closeFn, err := centrifuge.NewClient(stream.Context(), s.node, transport)
	if err != nil {
		return err
	}
	defer func() { _ = closeFn() }()

	s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection established", map[string]interface{}{"transport": transport.Name(), "client": c.ID()}))
	defer func(started time.Time) {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection completed", map[string]interface{}{"duration": time.Since(started), "transport": transport.Name(), "client": c.ID()}))
	}(time.Now())

	err = c.Connect(connectRequest)
	if err != nil {
		return err
	}

	for {
		select {
		case <-transport.closeCh:
			return nil
		case <-stream.Context().Done():
			return nil
		}
	}
}
