package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
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

func TestToClientErr(t *testing.T) {
	require.Equal(t, centrifuge.ErrorInternal, toClientErr(errors.New("boom")))
	require.Equal(t, centrifuge.ErrorUnknownChannel, toClientErr(centrifuge.ErrorUnknownChannel))
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

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{}, nil, false)
	require.Nil(t, reply.Error)
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Credentials)
}

func TestClientConnectingNoCredentialsNoTokenInsecure(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.ClientInsecure = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{}, nil, false)
	require.Nil(t, reply.Error)
	require.Nil(t, reply.Disconnect)
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

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{}, nil, false)
	require.Nil(t, reply.Error)
	require.Nil(t, reply.Disconnect)
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

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{
		Token: "bad bad token",
	}, nil, false)
	require.Equal(t, centrifuge.DisconnectInvalidToken, reply.Disconnect)
}

func TestClientConnectWithValidTokenHMAC(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", 0),
	}, nil, false)
	require.Nil(t, reply.Error)
	require.Nil(t, reply.Disconnect)
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

	transport := newTestTransport()

	connectProxyHandler := func(context.Context, centrifuge.TransportInfo, centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   "34",
				ExpireAt: 14,
				Info:     []byte(`{"key": "value"}`),
			},
			Channels: []string{"channel1"},
		}
	}

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{}, connectProxyHandler, true)
	require.Nil(t, reply.Error)
	require.Nil(t, reply.Disconnect)
	require.NotNil(t, reply.Credentials)
	require.Equal(t, "34", reply.Credentials.UserID)
	require.Equal(t, int64(14), reply.Credentials.ExpireAt)
	require.False(t, reply.ClientSideRefresh)
	require.True(t, stringInSlice("channel1", reply.Channels))
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

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{
		Token: getConnToken("42", 0, privateKey),
	}, nil, false)
	require.Nil(t, reply.Error)
	require.Nil(t, reply.Disconnect)
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

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", time.Now().Unix()+10),
	}, nil, false)
	require.Nil(t, reply.Error)
	require.Nil(t, reply.Disconnect)
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

	transport := newTestTransport()

	reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", 1525541722),
	}, nil, false)
	require.Equal(t, centrifuge.ErrorTokenExpired, reply.Error)
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

	reply := h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("42", 123),
	})
	require.True(t, reply.Expired)

	reply = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: "invalid",
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, reply.Disconnect)

	reply = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("42", 2525637058),
	})
	require.Nil(t, reply.Disconnect)
	require.False(t, reply.Expired)
	require.Equal(t, int64(2525637058), reply.ExpireAt)
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
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

	transport := newTestTransport()

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
			reply := h.OnClientConnecting(context.Background(), transport, centrifuge.ConnectEvent{
				Token: getConnTokenHS("42", 0),
			}, nil, false)
			stringInSlice(ruleContainer.PersonalChannel("42"), reply.Channels)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorUnknownChannel, reply.Error)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test#13",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$wrong_channel", "wrong client", 0),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)

	reply = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", "wrong client", 0),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)

	reply = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)

	reply = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "invalid",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)

	reply = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 123),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorTokenExpired, reply.Error)

	reply = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	})
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Error)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), time.Now().Unix()+10),
	})
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Error)
	require.True(t, reply.ExpireAt > 0)

	subRefreshReply := h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 123),
	})
	require.True(t, subRefreshReply.Expired)

	subRefreshReply = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   "invalid",
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, subRefreshReply.Disconnect)

	subRefreshReply = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 2525637058),
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, subRefreshReply.Disconnect)

	subRefreshReply = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 2525637058),
	})
	require.Nil(t, subRefreshReply.Disconnect)
	require.False(t, subRefreshReply.Expired)
	require.Equal(t, int64(2525637058), subRefreshReply.ExpireAt)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
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

	node.On().ClientConnecting(func(ctx context.Context, info centrifuge.TransportInfo, event centrifuge.ConnectEvent) centrifuge.ConnectReply {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}
	})

	connectCommand := &protocol.Command{
		ID: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := client.Handle(data)
	require.True(t, ok)

	reply := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", client.ID(), 0),
	})
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Error)
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

	reply := h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("$test1", "id", 10),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorTokenExpired, reply.Error)
}

func TestClientSubscribePermissionDeniedForAnonymous(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	reply := h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
}

func TestClientPublishNotAllowed(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}), proxy.Config{})

	reply := h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "non_existing_namespace:test1",
		Data:    []byte(`{}`),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorUnknownChannel, reply.Error)

	reply = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
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

	reply := h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	})
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Error)
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

	reply := h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
}

func TestClientHistory(t *testing.T) {
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
	require.NoError(t, client.Subscribe("test1"))

	reply := h.OnHistory(client, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Error)
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

	reply := h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorUnknownChannel, reply.Error)

	reply = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorNotAvailable, reply.Error)

	ruleConfig = ruleContainer.Config()
	ruleConfig.HistoryDisableForClient = false
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	reply = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
}

func TestClientPresence(t *testing.T) {
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
	require.NoError(t, client.Subscribe("test1"))

	reply := h.OnPresence(client, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Error)
}

func TestClientPresenceError(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.PresenceDisableForClient = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	reply := h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorUnknownChannel, reply.Error)

	reply = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorNotAvailable, reply.Error)

	ruleConfig = ruleContainer.Config()
	ruleConfig.PresenceDisableForClient = false
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	reply = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
}

func TestClientPresenceStats(t *testing.T) {
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
	require.NoError(t, client.Subscribe("test1"))

	reply := h.OnPresenceStats(client, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Nil(t, reply.Error)
}

func TestClientPresenceStatsError(t *testing.T) {
	node := nodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultRuleConfig
	ruleConfig.PresenceDisableForClient = true
	ruleContainer := rule.NewNamespaceRuleContainer(ruleConfig)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}), proxy.Config{})

	reply := h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorUnknownChannel, reply.Error)

	reply = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorNotAvailable, reply.Error)

	ruleConfig = ruleContainer.Config()
	ruleConfig.PresenceDisableForClient = false
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	reply = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Nil(t, reply.Disconnect)
	require.Equal(t, centrifuge.ErrorPermissionDenied, reply.Error)
}
