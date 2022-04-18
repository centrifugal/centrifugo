package tools

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"

	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
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

// ProtocolVersion returns transport protocol version.
func (t *TestTransport) ProtocolVersion() centrifuge.ProtocolVersion {
	return centrifuge.ProtocolVersion1
}

// Unidirectional - ...
func (t *TestTransport) Unidirectional() bool {
	return false
}

// DisabledPushFlags - ...
func (t *TestTransport) DisabledPushFlags() uint64 {
	return centrifuge.PushFlagDisconnect
}

// AppLevelPing ...
func (t *TestTransport) AppLevelPing() centrifuge.AppLevelPing {
	return centrifuge.AppLevelPing{
		PingInterval: 0,
	}
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

type CommonHTTPProxyTestCase struct {
	Node            *centrifuge.Node
	Client          *centrifuge.Client
	ClientCloseFunc centrifuge.ClientCloseFunc
	Server          *httptest.Server
	Mux             *http.ServeMux
}

func NewCommonHTTPProxyTestCase(ctx context.Context) *CommonHTTPProxyTestCase {
	node := NodeWithMemoryEngineNoHandlers()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	client, closeFn, err := centrifuge.NewClient(ctx, node, NewTestTransport())
	if err != nil {
		log.Fatalf("could not create centrifuge client: %v", err)
	}

	return &CommonHTTPProxyTestCase{
		Node:            node,
		Client:          client,
		ClientCloseFunc: closeFn,
		Server:          server,
		Mux:             mux,
	}
}

func (c *CommonHTTPProxyTestCase) Teardown() {
	defer func() { _ = c.Node.Shutdown(context.Background()) }()
	defer func() { _ = c.ClientCloseFunc() }()
	c.Server.Close()
}

type CommonGRPCProxyTestCase struct {
	Node            *centrifuge.Node
	Client          *centrifuge.Client
	ClientCloseFunc centrifuge.ClientCloseFunc
	Server          *grpc.Server
	Listener        *bufconn.Listener
}

func NewCommonGRPCProxyTestCase(ctx context.Context, srv proxyproto.CentrifugoProxyServer) *CommonGRPCProxyTestCase {
	node := NodeWithMemoryEngineNoHandlers()

	client, closeFn, err := centrifuge.NewClient(ctx, node, NewTestTransport())
	if err != nil {
		log.Fatalf("could not create centrifuge client: %v", err)
	}

	listener := bufconn.Listen(1024)
	server := grpc.NewServer()
	proxyproto.RegisterCentrifugoProxyServer(server, srv)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("GRPC server exited with error: %v", err)
		}
	}()

	return &CommonGRPCProxyTestCase{
		Node:            node,
		Client:          client,
		ClientCloseFunc: closeFn,
		Server:          server,
		Listener:        listener,
	}
}

func (c *CommonGRPCProxyTestCase) Teardown() {
	defer func() { _ = c.Node.Shutdown(context.Background()) }()
	defer func() { _ = c.ClientCloseFunc() }()
	c.Server.Stop()
}
