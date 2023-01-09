package unigrpc

import (
	"errors"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/unigrpc/unistream"

	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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

// NewService creates new Service.
func NewService(n *centrifuge.Node, c Config) *Service {
	if c.ProtocolVersion == 0 {
		c.ProtocolVersion = centrifuge.ProtocolVersion1
	}
	return &Service{
		config: c,
		node:   n,
	}
}

// Consume is a unidirectional server->client stream with real-time data.
func (s *Service) Consume(req *unistream.ConnectRequest, stream unistream.CentrifugoUniStream_ConsumeServer) error {
	protoVersion := s.config.ProtocolVersion
	if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
		mdProtocolVersion := md.Get("x-cf-protocol-version")
		if len(mdProtocolVersion) == 1 {
			queryProtocolVersion := mdProtocolVersion[0]
			if queryProtocolVersion != "" {
				switch queryProtocolVersion {
				case "v1":
					protoVersion = centrifuge.ProtocolVersion1
				case "v2":
					protoVersion = centrifuge.ProtocolVersion2
				default:
					s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelInfo, "unknown protocol version", map[string]interface{}{"transport": transportName, "version": queryProtocolVersion}))
					return errors.New("unknown protocol version")
				}
			}
		}
	}

	if centrifuge.DisableProtocolVersion1 && protoVersion == centrifuge.ProtocolVersion1 {
		return errors.New("unsupported protocol version")
	}

	streamDataCh := make(chan rawFrame)
	transport := newGRPCTransport(stream, streamDataCh, protoVersion)

	connectRequest := centrifuge.ConnectRequest{
		Token:   req.Token,
		Data:    req.Data,
		Name:    req.Name,
		Version: req.Version,
	}
	if req.Subs != nil {
		subs := make(map[string]centrifuge.SubscribeRequest, len(req.Subs))
		for k, v := range req.Subs {
			subs[k] = centrifuge.SubscribeRequest{
				Recover: v.Recover,
				Offset:  v.Offset,
				Epoch:   v.Epoch,
			}
		}
		connectRequest.Subs = subs
	}

	c, closeFn, err := centrifuge.NewClient(stream.Context(), s.node, transport)
	if err != nil {
		return err
	}
	defer func() { _ = closeFn() }()

	if s.node.LogEnabled(centrifuge.LogLevelDebug) {
		s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection established", map[string]interface{}{"transport": transport.Name(), "client": c.ID()}))
		defer func(started time.Time) {
			s.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelDebug, "client connection completed", map[string]interface{}{"duration": time.Since(started), "transport": transport.Name(), "client": c.ID()}))
		}(time.Now())
	}

	c.Connect(connectRequest)

	for {
		select {
		case <-transport.closeCh:
			return nil
		case <-stream.Context().Done():
			return nil
		}
	}
}
