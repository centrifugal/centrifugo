package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/internal/proxy"

	"github.com/centrifugal/centrifugo/internal/rule"
	rule2 "github.com/centrifugal/centrifugo/internal/rule"

	"github.com/centrifugal/centrifuge"

	"github.com/centrifugal/centrifugo/internal/jwtverify"

	"github.com/centrifugal/protocol"
	"github.com/cristalhq/jwt/v3"
	"github.com/stretchr/testify/require"
)

func generateTestRSAKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	reader := rand.Reader
	bitSize := 2048
	key, err := rsa.GenerateKey(reader, bitSize)
	require.NoError(t, err)
	return key, &key.PublicKey
}

func getTokenBuilder(rsaPrivateKey *rsa.PrivateKey) *jwt.Builder {
	var signer jwt.Signer
	if rsaPrivateKey != nil {
		signer, _ = jwt.NewSignerRS(jwt.RS256, rsaPrivateKey)
	} else {
		// For HS we do everything in tests with key `secret`.
		key := []byte(`secret`)
		signer, _ = jwt.NewSignerHS(jwt.HS256, key)

	}
	return jwt.NewBuilder(signer)
}

func getConnToken(user string, exp int64, rsaPrivateKey *rsa.PrivateKey) string {
	builder := getTokenBuilder(rsaPrivateKey)
	claims := &jwtverify.ConnectTokenClaims{
		Base64Info: "e30=",
		StandardClaims: jwt.StandardClaims{
			Subject: user,
		},
	}
	if exp > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Unix(exp, 0))
	}
	token, err := builder.Build(claims)
	if err != nil {
		panic(err)
	}
	return string(token.Raw())
}

func getSubscribeToken(channel string, client string, exp int64, rsaPrivateKey *rsa.PrivateKey) string {
	builder := getTokenBuilder(rsaPrivateKey)
	claims := &jwtverify.SubscribeTokenClaims{
		Base64Info:     "e30=",
		Channel:        channel,
		Client:         client,
		StandardClaims: jwt.StandardClaims{},
	}
	if exp > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Unix(exp, 0))
	}
	token, err := builder.Build(claims)
	if err != nil {
		panic(err)
	}
	return string(token.Raw())
}

func getConnTokenHS(user string, exp int64) string {
	return getConnToken(user, exp, nil)
}

func getSubscribeTokenHS(channel string, client string, exp int64) string {
	return getSubscribeToken(channel, client, exp, nil)
}

type testTransport struct {
	mu         sync.Mutex
	sink       chan []byte
	closed     bool
	closeCh    chan struct{}
	disconnect *centrifuge.Disconnect
	protoType  centrifuge.ProtocolType
}

func newTestTransport() *testTransport {
	return &testTransport{
		protoType: centrifuge.ProtocolTypeJSON,
		closeCh:   make(chan struct{}),
	}
}

func (t *testTransport) setProtocolType(pType centrifuge.ProtocolType) {
	t.protoType = pType
}

func (t *testTransport) setSink(sink chan []byte) {
	t.sink = sink
}

func (t *testTransport) Write(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return io.EOF
	}
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	if t.sink != nil {
		t.sink <- dataCopy
	}
	return nil
}

func (t *testTransport) Name() string {
	return "test_transport"
}

func (t *testTransport) Protocol() centrifuge.ProtocolType {
	return t.protoType
}

func (t *testTransport) Encoding() centrifuge.EncodingType {
	return centrifuge.EncodingTypeJSON
}

func (t *testTransport) Close(disconnect *centrifuge.Disconnect) error {
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

func nodeWithMemoryEngineNoHandlers() *centrifuge.Node {
	conf := centrifuge.DefaultConfig
	node, err := centrifuge.New(conf)
	if err != nil {
		panic(err)
	}
	err = node.Run()
	if err != nil {
		panic(err)
	}
	return node
}

func nodeWithMemoryEngine() *centrifuge.Node {
	n := nodeWithMemoryEngineNoHandlers()
	n.On().ClientConnected(func(ctx context.Context, client *centrifuge.Client) {
		client.On().Subscribe(func(_ centrifuge.SubscribeEvent) centrifuge.SubscribeReply {
			return centrifuge.SubscribeReply{}
		})
		client.On().Publish(func(_ centrifuge.PublishEvent) centrifuge.PublishReply {
			return centrifuge.PublishReply{}
		})
	})
	return n
}

func TestClientConnectNoCredentialsNoTokenInsecure(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientInsecure = true
	ruleContainer := rule2.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})
	h.Setup()

	transport := newTestTransport()
	client, closeFn, _ := centrifuge.NewClient(context.Background(), node, transport)
	defer func() { _ = closeFn() }()
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{}, rw)
	require.Nil(t, disconnect)
	require.Nil(t, replies[0].Error)
	result := extractConnectResult(replies, client.Transport().Protocol())
	require.NotEmpty(t, result.Client)
	require.Empty(t, client.UserID())
}

func TestClientConnectNoCredentialsNoTokenAnonymous(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	rule.config.ClientAnonymous = true
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{}, rw)
	require.Nil(t, disconnect)
	require.Nil(t, replies[0].Error)
	result := extractConnectResult(replies, client.Transport().Protocol())
	require.NotEmpty(t, result.Client)
	require.Empty(t, client.UserID())
}

func TestClientConnectWithMalformedToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{
		Token: "bad bad token",
	}, rw)
	require.NotNil(t, disconnect)
	require.Equal(t, disconnect, DisconnectInvalidToken)
}

func TestClientConnectWithValidTokenHMAC(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{
		Token: getConnTokenHS("42", 0),
	}, rw)
	require.Nil(t, disconnect)
	result := extractConnectResult(replies, client.Transport().Protocol())
	require.Equal(t, client.ID(), result.Client)
	require.Equal(t, false, result.Expires)
}

func TestClientConnectWithValidTokenRSA(t *testing.T) {
	privateKey, pubKey := generateTestRSAKeys(t)

	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{RSAPublicKey: pubKey}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{
		Token: getConnToken("42", 0, privateKey),
	}, rw)
	require.Nil(t, disconnect)
	result := extractConnectResult(replies, client.Transport().Protocol())
	require.Equal(t, client.ID(), result.Client)
	require.Equal(t, false, result.Expires)
}

func TestClientConnectWithExpiringToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return ConnectReply{
			ClientSideRefresh: true,
		}
	})

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{
		Token: getConnTokenHS("42", time.Now().Unix()+10),
	}, rw)
	require.Nil(t, disconnect)
	result := extractConnectResult(replies, client.Transport().Protocol())
	require.Equal(t, true, result.Expires)
	require.True(t, result.TTL > 0)
	require.True(t, client.authenticated)
}

func TestClientConnectWithExpiredToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{
		Token: getConnTokenHS("42", 1525541722),
	}, rw)
	require.Nil(t, disconnect)
	require.Equal(t, ErrorTokenExpired.toProto(), replies[0].Error)
	require.False(t, client.authenticated)
}

func TestClientSideTokenRefresh(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})
	node.On().ClientConnected(func(ctx context.Context, client *Client) {
		client.On().Refresh(func(event RefreshEvent) RefreshReply {
			return h.OnRefresh(client, event)
		})
	})

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)
	disconnect := client.connectCmd(&protocol.ConnectRequest{
		Token: getConnTokenHS("42", 2525541722),
	}, rw)
	require.Nil(t, disconnect)
	require.Nil(t, replies[0].Error)
	client.triggerConnect()

	refreshResp, disconnect := client.refreshCmd(&protocol.RefreshRequest{
		Token: getConnTokenHS("42", 2525637058),
	})
	require.Nil(t, disconnect)
	require.NotEmpty(t, client.ID())
	require.True(t, refreshResp.Result.Expires)
	require.True(t, refreshResp.Result.TTL > 0)
}

func TestClientUserPersonalChannel(t *testing.T) {
	node := nodeWithMemoryEngine()

	ruleConfig := DefaultRuleConfig
	ruleConfig.UserSubscribeToPersonal = true
	ruleConfig.Namespaces = []ChannelNamespace{
		{
			Name:                    "user",
			NamespaceChannelOptions: NamespaceChannelOptions{},
		},
	}
	rule := NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))

	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})
	defer func() { _ = node.Shutdown(context.Background()) }()

	var tests = []struct {
		Name      string
		Namespace string
		Error     *Error
	}{
		{"ok_no_namespace", "", nil},
		{"ok_with_namespace", "user", nil},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			config := rule.Config()
			config.UserSubscribeToPersonal = true
			config.UserPersonalChannelNamespace = tt.Namespace
			err := rule.Reload(config)
			require.NoError(t, err)
			transport := newTestTransport()
			transport.sink = make(chan []byte, 100)
			ctx := context.Background()
			newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
			client, _ := newClient(newCtx, node, transport)
			var replies []*protocol.Reply
			rw := testReplyWriter(&replies)
			disconnect := client.connectCmd(&protocol.ConnectRequest{
				Token: getConnTokenHS("42", 2525541722),
			}, rw)
			require.Nil(t, disconnect)
			require.Nil(t, replies[0].Error)
			if tt.Error != nil {
				require.Equal(t, tt.Error, replies[0].Error)
			} else {
				done := make(chan struct{})
				go func() {
					for data := range transport.sink {
						if strings.Contains(string(data), "test message") {
							close(done)
						}
					}
				}()

				_, err := node.Publish(rule.personalChannel("42"), []byte(`{"text": "test message"}`))
				require.NoError(t, err)

				select {
				case <-time.After(time.Second):
					require.Fail(t, "timeout receiving publication")
				case <-done:
				}
			}
		})
	}
}

func TestClientSubscribePrivateChannelNoToken(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})
	node.On().ClientConnected(func(ctx context.Context, client *Client) {
		client.On().Subscribe(func(event SubscribeEvent) SubscribeReply {
			return h.OnSubscribe(client, event)
		})
	})

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)

	subCtx := client.subscribeCmd(&protocol.SubscribeRequest{
		Channel: "$test1",
	}, rw, false)
	require.Nil(t, subCtx.disconnect)
	require.Equal(t, ErrorPermissionDenied.toProto(), replies[0].Error)
}

func TestClientSubscribePrivateChannelWithToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})
	node.On().ClientConnected(func(ctx context.Context, client *Client) {
		client.On().Subscribe(func(event SubscribeEvent) SubscribeReply {
			return h.OnSubscribe(client, event)
		})
	})

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)

	subCtx := client.subscribeCmd(&protocol.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$wrong_channel", "wrong client", 0),
	}, rw, false)
	require.Nil(t, subCtx.disconnect)
	require.Equal(t, ErrorPermissionDenied.toProto(), replies[0].Error)

	replies = nil
	subCtx = client.subscribeCmd(&protocol.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$wrong_channel", client.ID(), 0),
	}, rw, false)
	require.Nil(t, subCtx.disconnect)
	require.Equal(t, ErrorPermissionDenied.toProto(), replies[0].Error)

	replies = nil
	subCtx = client.subscribeCmd(&protocol.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	}, rw, false)
	require.Nil(t, subCtx.disconnect)
	require.Nil(t, replies[0].Error)
}

func TestClientSubscribePrivateChannelWithExpiringToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})
	node.On().ClientConnected(func(ctx context.Context, client *Client) {
		client.On().Subscribe(func(event SubscribeEvent) SubscribeReply {
			return h.OnSubscribe(client, event)
		})
	})

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	var replies []*protocol.Reply
	rw := testReplyWriter(&replies)

	subCtx := client.subscribeCmd(&protocol.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 10),
	}, rw, false)
	require.Nil(t, subCtx.disconnect)
	require.Equal(t, ErrorTokenExpired.toProto(), replies[0].Error)

	replies = nil
	subCtx = client.subscribeCmd(&protocol.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), time.Now().Unix()+10),
	}, rw, false)
	require.Nil(t, subCtx.disconnect)
	require.Nil(t, replies[0].Error, "token is valid and not expired yet")
	res := extractSubscribeResult(replies, client.Transport().Protocol())
	require.True(t, res.Expires, "expires flag must be set")
	require.True(t, res.TTL > 0, "positive TTL must be set")
}

func TestClientPublish(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	publishResp, disconnect := client.publishCmd(&protocol.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	require.Nil(t, disconnect)
	require.Equal(t, ErrorNotAvailable.toProto(), publishResp.Error)

	rule := NewNamespaceRuleContainer(DefaultRuleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})

	client.On().Publish(func(event PublishEvent) PublishReply {
		return h.OnPublish(client, event)
	})

	client.On().Subscribe(func(event SubscribeEvent) SubscribeReply {
		return h.OnSubscribe(client, event)
	})

	config := rule.Config()
	config.Publish = true
	_ = rule.Reload(config)

	publishResp, disconnect = client.publishCmd(&protocol.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	require.Nil(t, disconnect)
	require.Nil(t, publishResp.Error)

	config = rule.Config()
	config.SubscribeToPublish = true
	_ = rule.Reload(config)

	publishResp, disconnect = client.publishCmd(&protocol.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	require.Nil(t, disconnect)
	require.Equal(t, ErrorPermissionDenied.toProto(), publishResp.Error)

	subscribeClient(t, client, "test")
	publishResp, disconnect = client.publishCmd(&protocol.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	require.Nil(t, disconnect)
	require.Nil(t, publishResp.Error)
}

func TestClientHistoryDisabled(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	config := node.Config()
	setTestChannelOptions(&config, ChannelOptions{
		HistorySize:     10,
		HistoryLifetime: 60,
	})
	_ = node.Reload(config)

	ruleConfig := DefaultRuleConfig
	ruleConfig.HistoryDisableForClient = true
	rule := NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})
	node.On().ClientConnected(func(ctx context.Context, client *Client) {
		client.On().Subscribe(func(event SubscribeEvent) SubscribeReply {
			return h.OnSubscribe(client, event)
		})
	})

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	_, _ = node.Publish("test", []byte(`{}`))

	connectClient(t, client)
	subscribeClient(t, client, "test")

	historyResp, disconnect := client.historyCmd(&protocol.HistoryRequest{
		Channel: "test",
	})
	require.Nil(t, disconnect)
	require.Equal(t, ErrorNotAvailable.toProto(), historyResp.Error)

	client.On().History(func(event HistoryEvent) HistoryReply {
		return h.OnHistory(client, event)
	})

	historyResp, disconnect = client.historyCmd(&protocol.HistoryRequest{
		Channel: "test",
	})
	require.Nil(t, disconnect)
	require.Equal(t, ErrorNotAvailable.toProto(), historyResp.Error)
}

func TestClientPresenceDisabled(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	config := node.Config()
	setTestChannelOptions(&config, ChannelOptions{
		Presence: true,
	})
	_ = node.Reload(config)

	ruleConfig := DefaultRuleConfig
	ruleConfig.PresenceDisableForClient = true
	rule := NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, rule, NewTokenVerifierJWT(TokenVerifierConfig{HMACSecretKey: "secret"}))
	node.On().ClientConnecting(func(ctx context.Context, info TransportInfo, event ConnectEvent) ConnectReply {
		return h.OnConnecting(ctx, info, event)
	})
	node.On().ClientConnected(func(ctx context.Context, client *Client) {
		client.On().Subscribe(func(event SubscribeEvent) SubscribeReply {
			return h.OnSubscribe(client, event)
		})
	})

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	_, _ = node.Publish("test", []byte(`{}`))

	connectClient(t, client)
	subscribeClient(t, client, "test")

	presenceResp, disconnect := client.presenceCmd(&protocol.PresenceRequest{
		Channel: "test",
	})
	require.Nil(t, disconnect)
	require.Equal(t, ErrorNotAvailable.toProto(), presenceResp.Error)

	client.On().Presence(func(event PresenceEvent) PresenceReply {
		return h.OnPresence(client, event)
	})

	presenceResp, disconnect = client.presenceCmd(&protocol.PresenceRequest{
		Channel: "test",
	})
	require.Nil(t, disconnect)
	require.Equal(t, ErrorNotAvailable.toProto(), presenceResp.Error)
}
