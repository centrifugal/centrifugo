package client

import (
	"context"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/proxy"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
)

// benchHandler is a handler for the client command paths: tokens are HMAC,
// channels without namespace allow clients to subscribe and publish and keep
// history; namespace px has subscribe and publish proxies enabled.
func benchHandler(b *testing.B) *Handler {
	b.Helper()
	node := tools.NodeWithMemoryEngineNoHandlers()
	b.Cleanup(func() { _ = node.Shutdown(context.Background()) })
	cfg := config.DefaultConfig()
	o := &cfg.Channel.WithoutNamespace
	o.SubscribeForClient = true
	o.PublishForClient = true
	o.HistorySize = 10
	o.HistoryTTL = configtypes.Duration(time.Minute)
	cfg.Channel.Proxy.Subscribe = configtypes.Proxy{Endpoint: "http://localhost:3000/subscribe", Timeout: configtypes.Duration(time.Second)}
	cfg.Channel.Proxy.Publish = configtypes.Proxy{Endpoint: "http://localhost:3000/publish", Timeout: configtypes.Duration(time.Second)}
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
		Name: "px",
		ChannelOptions: configtypes.ChannelOptions{
			SubscribeProxyEnabled: true,
			SubscribeProxyName:    "default",
			PublishProxyEnabled:   true,
			PublishProxyName:      "default",
		},
	}}
	cfgContainer, err := config.NewContainer(cfg)
	if err != nil {
		b.Fatal(err)
	}
	verifier, err := jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{HMACSecretKey: "secret"}, cfgContainer)
	if err != nil {
		b.Fatal(err)
	}
	return NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})
}

func benchClient() *tools.TestClientMock {
	return &tools.TestClientMock{
		IDFunc:           func() string { return "client" },
		UserIDFunc:       func() string { return "12" },
		IsSubscribedFunc: func(string) bool { return false },
		ContextFunc:      func() context.Context { return context.Background() },
	}
}

func BenchmarkHandlerConnectToken(b *testing.B) {
	h := benchHandler(b)
	e := centrifuge.ConnectEvent{ClientID: "client", Token: getConnTokenHS("12", time.Now().Unix()+3600)}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.OnClientConnecting(ctx, e, nil, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandlerConnectProxy(b *testing.B) {
	h := benchHandler(b)
	e := centrifuge.ConnectEvent{ClientID: "client"}
	ctx := context.Background()
	credentials := &centrifuge.Credentials{UserID: "12"}
	connectProxy := func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectReply, proxy.ConnectExtra, error) {
		return centrifuge.ConnectReply{Credentials: credentials}, proxy.ConnectExtra{}, nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.OnClientConnecting(ctx, e, connectProxy, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandlerRefresh(b *testing.B) {
	h := benchHandler(b)
	c := benchClient()
	e := centrifuge.RefreshEvent{Token: getConnTokenHS("12", time.Now().Unix()+3600)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := h.OnRefresh(c, e, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func benchSubscribe(b *testing.B, e centrifuge.SubscribeEvent) {
	h := benchHandler(b)
	c := benchClient()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := h.OnSubscribe(c, e, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandlerSubscribe(b *testing.B) {
	benchSubscribe(b, centrifuge.SubscribeEvent{Channel: "a"})
}

func BenchmarkHandlerSubscribeToken(b *testing.B) {
	benchSubscribe(b, centrifuge.SubscribeEvent{Channel: "$a", Token: getSubscribeTokenHS("12", "$a", time.Now().Unix()+3600)})
}

func BenchmarkHandlerSubRefresh(b *testing.B) {
	h := benchHandler(b)
	c := benchClient()
	e := centrifuge.SubRefreshEvent{Channel: "$a", Token: getSubscribeTokenHS("12", "$a", time.Now().Unix()+3600)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := h.OnSubRefresh(c, nil, e); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandlerPublish(b *testing.B) {
	h := benchHandler(b)
	c := benchClient()
	e := centrifuge.PublishEvent{Channel: "a", Data: []byte(`{}`)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.OnPublish(c, e, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandlerSubscribeProxy(b *testing.B) {
	h := benchHandler(b)
	c := benchClient()
	e := centrifuge.SubscribeEvent{Channel: "px:a"}
	subscribeProxy := func(proxy.Client, centrifuge.SubscribeEvent, configtypes.ChannelOptions, proxy.PerCallData) (centrifuge.SubscribeReply, proxy.SubscribeExtra, error) {
		return centrifuge.SubscribeReply{}, proxy.SubscribeExtra{}, nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := h.OnSubscribe(c, e, subscribeProxy, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHandlerPublishProxy(b *testing.B) {
	h := benchHandler(b)
	c := benchClient()
	e := centrifuge.PublishEvent{Channel: "px:a", Data: []byte(`{}`)}
	publishProxy := func(proxy.Client, centrifuge.PublishEvent, configtypes.ChannelOptions, proxy.PerCallData) (centrifuge.PublishReply, error) {
		return centrifuge.PublishReply{Result: &centrifuge.PublishResult{}}, nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := h.OnPublish(c, e, publishProxy); err != nil {
			b.Fatal(err)
		}
	}
}
