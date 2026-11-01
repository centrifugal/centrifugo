package proxy

import (
	"context"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
)

// benchProxy answers every proxy call with a fixed response, without network.
type benchProxy struct {
	connect    *proxyproto.ConnectResponse
	refresh    *proxyproto.RefreshResponse
	subscribe  *proxyproto.SubscribeResponse
	subRefresh *proxyproto.SubRefreshResponse
}

func (p *benchProxy) Name() string      { return "default" }
func (p *benchProxy) Protocol() string  { return "bench" }
func (p *benchProxy) UseBase64() bool   { return false }
func (p *benchProxy) IncludeMeta() bool { return false }

func (p *benchProxy) ProxyConnect(context.Context, *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	return p.connect, nil
}

func (p *benchProxy) ProxyRefresh(context.Context, *proxyproto.RefreshRequest) (*proxyproto.RefreshResponse, error) {
	return p.refresh, nil
}

func (p *benchProxy) ProxySubscribe(context.Context, *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	return p.subscribe, nil
}

func (p *benchProxy) ProxySubRefresh(context.Context, *proxyproto.SubRefreshRequest) (*proxyproto.SubRefreshResponse, error) {
	return p.subRefresh, nil
}

func BenchmarkProxyHandlers(b *testing.B) {
	cfgContainer, err := config.NewContainer(config.DefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	p := &benchProxy{
		connect: &proxyproto.ConnectResponse{Result: &proxyproto.ConnectResult{
			User: "42", Channels: []string{"a"}, Subs: map[string]*proxyproto.SubscribeOptions{"b": {}},
		}},
		refresh:    &proxyproto.RefreshResponse{Result: &proxyproto.RefreshResult{ExpireAt: 1}},
		subscribe:  &proxyproto.SubscribeResponse{Result: &proxyproto.SubscribeResult{}},
		subRefresh: &proxyproto.SubRefreshResponse{Result: &proxyproto.SubRefreshResult{ExpireAt: 1}},
	}
	client := &tools.TestClientMock{
		IDFunc:        func() string { return "client-id" },
		UserIDFunc:    func() string { return "42" },
		ContextFunc:   context.Background,
		TransportFunc: func() centrifuge.TransportInfo { return tools.NewTestTransport() },
	}
	chOpts := configtypes.ChannelOptions{
		SubscribeProxyEnabled:  true,
		SubscribeProxyName:     "default",
		SubRefreshProxyEnabled: true,
		SubRefreshProxyName:    "default",
	}

	b.Run("connect", func(b *testing.B) {
		h := NewConnectHandler(ConnectHandlerConfig{Proxy: p}, cfgContainer).Handle()
		e := centrifuge.ConnectEvent{ClientID: "client-id", Transport: tools.NewTestTransport()}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := h(context.Background(), e); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("refresh", func(b *testing.B) {
		h := NewRefreshHandler(RefreshHandlerConfig{Proxy: p}).Handle()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := h(client, centrifuge.RefreshEvent{}, PerCallData{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("subscribe", func(b *testing.B) {
		h := NewSubscribeHandler(SubscribeHandlerConfig{Proxies: map[string]SubscribeProxy{"default": p}}).Handle()
		e := centrifuge.SubscribeEvent{Channel: "ch"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := h(client, e, chOpts, PerCallData{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("sub_refresh", func(b *testing.B) {
		h := NewSubRefreshHandler(SubRefreshHandlerConfig{Proxies: map[string]SubRefreshProxy{"default": p}}).Handle()
		e := centrifuge.SubRefreshEvent{Channel: "ch"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := h(client, e, chOpts, PerCallData{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
