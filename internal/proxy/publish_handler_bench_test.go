package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
)

// staticPublishProxy answers every publish request with the same response,
// without a network round trip.
type staticPublishProxy struct {
	resp *proxyproto.PublishResponse
}

func (p *staticPublishProxy) ProxyPublish(context.Context, *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
	return p.resp, nil
}
func (p *staticPublishProxy) Protocol() string  { return "static" }
func (p *staticPublishProxy) UseBase64() bool   { return false }
func (p *staticPublishProxy) IncludeMeta() bool { return false }

func BenchmarkPublishHandler(b *testing.B) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	b.Cleanup(func() { _ = node.Shutdown(context.Background()) })
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, tools.NewTestTransport())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = closeFn() })

	handler := NewPublishHandler(PublishHandlerConfig{
		Proxies: map[string]PublishProxy{
			"test": &staticPublishProxy{resp: &proxyproto.PublishResponse{Result: &proxyproto.PublishResult{}}},
		},
	}).Handle(node)
	chOpts := configtypes.ChannelOptions{
		HistoryTTL:          configtypes.Duration(time.Minute),
		HistorySize:         10,
		PublishProxyEnabled: true,
		PublishProxyName:    "test",
	}
	e := centrifuge.PublishEvent{Channel: "test", Data: []byte(`{"k":"v"}`)}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := handler(client, e, chOpts, PerCallData{})
		if err != nil {
			b.Fatal(err)
		}
	}
}
