package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/internal/jwtverify"
	"github.com/centrifugal/centrifugo/internal/proxy"
	"github.com/centrifugal/centrifugo/internal/rule"

	"github.com/centrifugal/centrifuge"
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
	n.OnConnect(func(client *centrifuge.Client) {
		client.OnSubscribe(func(_ centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			cb(centrifuge.SubscribeResult{}, nil)
		})
		client.OnPublish(func(_ centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			cb(centrifuge.PublishResult{}, nil)
		})
	})
	return n
}

func TestClientHandlerSetup(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientInsecure = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})
	h.Setup()

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)
}

func TestClientConnectingNoCredentialsNoToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Transport: newTestTransport(),
	}, nil, false)
	require.NoError(t, err)
	require.Nil(t, reply.Credentials)
}

func TestClientConnectingNoCredentialsNoTokenInsecure(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientInsecure = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "", reply.Credentials.UserID)
}

func TestClientConnectNoCredentialsNoTokenAnonymous(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientAnonymous = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "", reply.Credentials.UserID)
}

func TestClientConnectWithMalformedToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: "bad bad token",
	}, nil, false)
	require.Error(t, err)
}

func TestClientConnectWithValidTokenHMAC(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", 0),
	}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "42", reply.Credentials.UserID)
	require.Equal(t, int64(0), reply.Credentials.ExpireAt)
}

func TestClientConnectWithProxy(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	connectProxyHandler := func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID:   "34",
				ExpireAt: 14,
				Info:     []byte(`{"key": "value"}`),
			},
			Subscriptions: []centrifuge.Subscription{{Channel: "channel1"}},
		}, nil
	}

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, connectProxyHandler, true)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "34", reply.Credentials.UserID)
	require.Equal(t, int64(14), reply.Credentials.ExpireAt)
	require.False(t, reply.ClientSideRefresh)
	require.Equal(t, "channel1", reply.Subscriptions[0].Channel)
}

func TestClientConnectWithValidTokenRSA(t *testing.T) {
	privateKey, pubKey := generateTestRSAKeys(t)

	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		RSAPublicKey: pubKey,
	}), proxy.Config{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnToken("42", 0, privateKey),
	}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "42", reply.Credentials.UserID)
	require.Equal(t, int64(0), reply.Credentials.ExpireAt)
}

func TestClientConnectWithExpiringToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientAnonymous = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", time.Now().Unix()+10),
	}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "42", reply.Credentials.UserID)
	require.True(t, reply.Credentials.ExpireAt > 0)
}

func TestClientConnectWithExpiredToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientAnonymous = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", 1525541722),
	}, nil, false)
	require.Equal(t, centrifuge.ErrorTokenExpired, err)
}

func TestClientSideRefresh(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientAnonymous = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	reply, err := h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("42", 123),
	})
	require.NoError(t, err)
	require.True(t, reply.Expired)

	_, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: "invalid",
	})
	require.Error(t, err)

	reply, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("42", 2525637058),
	})
	require.NoError(t, err)
	require.False(t, reply.Expired)
	require.Equal(t, int64(2525637058), reply.ExpireAt)
}

func TestClientUserPersonalChannel(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.UserSubscribeToPersonal = true
	ruleConfig.Namespaces = []rule.ChannelNamespace{
		{
			Name:                    "user",
			NamespaceChannelOptions: rule.NamespaceChannelOptions{},
		},
	}
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	var tests = []struct {
		Name      string
		Namespace string
	}{
		{"ok_no_namespace", ""},
		{"ok_with_namespace", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			config := ruleContainer.Config()
			config.UserSubscribeToPersonal = true
			config.UserPersonalChannelNamespace = tt.Namespace
			err := ruleContainer.Reload(config)
			require.NoError(t, err)
			reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
				Token: getConnTokenHS("42", 0),
			}, nil, false)
			require.NoError(t, err)
			require.Equal(t, ruleContainer.PersonalChannel("42"), reply.Subscriptions[0].Channel)
		})
	}
}

func TestClientSubscribeChannel(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "non_existing_namespace:test1",
	}, nil)
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)
}

func TestClientSubscribeChannelServerSide(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ServerSide = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test1",
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSubscribeChannelUserLimited(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test#13",
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSubscribePrivateChannelWithToken(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$wrong_channel", "wrong client", 0),
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", "wrong client", 0),
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "",
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "invalid",
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 123),
	}, nil)
	require.Equal(t, centrifuge.ErrorTokenExpired, err)

	reply, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	}, nil)
	require.NoError(t, err)
	require.Zero(t, reply.ExpireAt)
}

func TestClientSubscribePrivateChannelWithTokenAnonymous(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSideSubRefresh(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientAnonymous = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), time.Now().Unix()+10),
	}, nil)
	require.NoError(t, err)
	require.True(t, reply.ExpireAt > 0)

	subRefreshResult, err := h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 123),
	})
	require.NoError(t, err)
	require.True(t, subRefreshResult.Expired)

	subRefreshResult, err = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   "invalid",
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, err)

	subRefreshResult, err = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 2525637058),
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, err)

	subRefreshResult, err = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 2525637058),
	})
	require.NoError(t, err)
	require.False(t, subRefreshResult.Expired)
	require.Equal(t, int64(2525637058), subRefreshResult.ExpireAt)
}

func TestClientSubscribePrivateChannelWithTokenAnonymousNotAllowed(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	_, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSubscribePrivateChannelWithTokenAnonymousAllowed(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.Anonymous = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectResult, error) {
		return centrifuge.ConnectResult{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	}, nil)
	require.NoError(t, err)
	require.Zero(t, reply.ExpireAt)
}

func TestClientSubscribePrivateChannelWithExpiringToken(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.Anonymous = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", "id", 10),
	}, nil)
	require.Equal(t, centrifuge.ErrorTokenExpired, err)
}

func TestClientSubscribePermissionDeniedForAnonymous(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "test1",
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPublishNotAllowed(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "non_existing_namespace:test1",
		Data:    []byte(`{}`),
	}, nil)
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPublishAllowed(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.Publish = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil)
	require.NoError(t, err)
}

func TestClientSubscribeToPublish(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.Publish = true
	ruleConfig.SubscribeToPublish = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientHistory(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.HistorySize = 10
	ruleConfig.HistoryLifetime = 300
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()
	require.NoError(t, client.Subscribe("test1"))

	_, err = h.OnHistory(client, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.NoError(t, err)
}

func TestClientHistoryError(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.HistoryDisableForClient = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	_, err := h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	ruleConfig = ruleContainer.Config()
	ruleConfig.HistoryDisableForClient = false
	ruleConfig.HistorySize = 10
	ruleConfig.HistoryLifetime = 300
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPresence(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.Presence = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()
	require.NoError(t, client.Subscribe("test1"))

	_, err = h.OnPresence(client, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.NoError(t, err)
}

func TestClientPresenceError(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.PresenceDisableForClient = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	_, err := h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	ruleConfig = ruleContainer.Config()
	ruleConfig.PresenceDisableForClient = false
	ruleConfig.Presence = true
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPresenceStats(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.Presence = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()
	require.NoError(t, client.Subscribe("test1"))

	_, err = h.OnPresenceStats(client, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.NoError(t, err)
}

func TestClientPresenceStatsError(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.PresenceDisableForClient = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	_, err := h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	ruleConfig = ruleContainer.Config()
	ruleConfig.PresenceDisableForClient = false
	ruleConfig.Presence = true
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}
