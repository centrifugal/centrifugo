package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v4/internal/proxy"
	"github.com/centrifugal/centrifugo/v4/internal/rule"
	"github.com/centrifugal/centrifugo/v4/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/cristalhq/jwt/v4"
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
		RegisteredClaims: jwt.RegisteredClaims{
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
	return token.String()
}

func getSubscribeToken(user string, channel string, exp int64, rsaPrivateKey *rsa.PrivateKey) string {
	builder := getTokenBuilder(rsaPrivateKey)
	claims := &jwtverify.SubscribeTokenClaims{
		SubscribeOptions: jwtverify.SubscribeOptions{
			Base64Info: "e30=",
		},
		Channel: channel,
		RegisteredClaims: jwt.RegisteredClaims{
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
	return token.String()
}

func getConnTokenHS(user string, exp int64) string {
	return getConnToken(user, exp, nil)
}

func getSubscribeTokenHS(user string, channel string, exp int64) string {
	return getSubscribeToken(user, channel, exp, nil)
}

func TestClientHandlerSetup(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.ClientInsecure = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}, ruleContainer), &ProxyMap{}, false)
	err = h.Setup()
	require.NoError(t, err)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)
}

func TestClientConnectingNoCredentialsNoToken(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}, ruleContainer), &ProxyMap{}, false)

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Transport: tools.NewTestTransport(),
	}, nil, false)
	require.NoError(t, err)
	require.Nil(t, reply.Credentials)
}

func TestClientConnectingNoCredentialsNoTokenInsecure(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.ClientInsecure = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}, ruleContainer), &ProxyMap{}, false)

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "", reply.Credentials.UserID)
}

func TestClientConnectNoCredentialsNoTokenAnonymous(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.AnonymousConnectWithoutToken = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}, ruleContainer), &ProxyMap{}, false)

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "", reply.Credentials.UserID)
}

func TestClientConnectWithMalformedToken(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: "bad bad token",
	}, nil, false)
	require.Error(t, err)
}

func TestClientConnectWithValidTokenHMAC(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", 0),
	}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "42", reply.Credentials.UserID)
	require.Equal(t, int64(0), reply.Credentials.ExpireAt)
}

func TestClientConnectWithProxy(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	connectProxyHandler := func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectReply, proxy.ConnectExtra, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID:   "34",
				ExpireAt: 14,
				Info:     []byte(`{"key": "value"}`),
			},
			Subscriptions: map[string]centrifuge.SubscribeOptions{"channel1": {}},
		}, proxy.ConnectExtra{}, nil
	}

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, connectProxyHandler, true)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "34", reply.Credentials.UserID)
	require.Equal(t, int64(14), reply.Credentials.ExpireAt)
	require.False(t, reply.ClientSideRefresh)
	require.Contains(t, reply.Subscriptions, "channel1")
}

func TestClientConnectWithValidTokenRSA(t *testing.T) {
	privateKey, pubKey := generateTestRSAKeys(t)

	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		RSAPublicKey: pubKey,
	}, ruleContainer), &ProxyMap{}, false)

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnToken("42", 0, privateKey),
	}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "42", reply.Credentials.UserID)
	require.Equal(t, int64(0), reply.Credentials.ExpireAt)
}

func TestClientConnectWithExpiringToken(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.AnonymousConnectWithoutToken = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", time.Now().Unix()+10),
	}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "42", reply.Credentials.UserID)
	require.True(t, reply.Credentials.ExpireAt > 0)
}

func TestClientConnectWithExpiredToken(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.AnonymousConnectWithoutToken = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", 1525541722),
	}, nil, false)
	require.Equal(t, centrifuge.ErrorTokenExpired, err)
}

func TestClientSideRefresh(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.AnonymousConnectWithoutToken = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	reply, _, err := h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("", 123),
	}, nil, proxy.PerCallData{})
	require.NoError(t, err)
	require.True(t, reply.Expired)

	_, _, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: "invalid",
	}, nil, proxy.PerCallData{})
	require.Error(t, err)

	reply, _, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("", 2525637058),
	}, nil, proxy.PerCallData{})
	require.NoError(t, err)
	require.False(t, reply.Expired)
	require.Equal(t, int64(2525637058), reply.ExpireAt)
}

func TestClientSideRefreshDifferentUser(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.AnonymousConnectWithoutToken = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, _, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("42", 2525637058),
	}, nil, proxy.PerCallData{})
	require.ErrorIs(t, err, centrifuge.DisconnectInvalidToken)
}

func TestClientUserPersonalChannel(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.UserSubscribeToPersonal = true
	ruleConfig.Namespaces = []rule.ChannelNamespace{
		{
			Name:           "user",
			ChannelOptions: rule.ChannelOptions{},
		},
	}
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

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
			require.Contains(t, reply.Subscriptions, ruleContainer.PersonalChannel("42"))
		})
	}
}

func TestClientSubscribeChannel(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "non_existing_namespace:test1",
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)
}

func TestClientSubscribeChannelNoPermission(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test1",
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSubscribeChannelUserLimitedError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.UserLimitedChannels = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test#13",
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSubscribeChannelUserLimitedOK(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.UserLimitedChannels = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test#12",
	}, nil, proxy.PerCallData{})
	require.NoError(t, err)
}

func TestClientSubscribeWithToken(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "",
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "invalid",
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", 123),
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorTokenExpired, err)

	reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", 0),
	}, nil, proxy.PerCallData{})
	require.NoError(t, err)
	require.Zero(t, reply.Options.ExpireAt)
}

func TestClientSubscribeWithTokenAnonymous(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("", "$test1", 0),
	}, nil, proxy.PerCallData{})
	require.NoError(t, err)
}

func TestClientSideSubRefresh(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.AnonymousConnectWithoutToken = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "12",
			},
		}, nil
	})

	connectCommand := &protocol.Command{
		Id: 1,
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", time.Now().Unix()+10),
	}, nil, proxy.PerCallData{})
	require.NoError(t, err)
	require.True(t, reply.Options.ExpireAt > 0)

	subRefreshReply, _, err := h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("12", "$test1", 123),
	})
	require.NoError(t, err)
	require.True(t, subRefreshReply.Expired)

	subRefreshReply, _, err = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   "invalid",
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, err)

	subRefreshReply, _, err = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("12", "$test1", 2525637058),
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, err)

	subRefreshReply, _, err = h.OnSubRefresh(client, centrifuge.SubRefreshEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", 2525637058),
	})
	require.NoError(t, err)
	require.False(t, subRefreshReply.Expired)
	require.Equal(t, int64(2525637058), subRefreshReply.ExpireAt)
}

func TestClientSubscribePrivateChannelWithExpiringToken(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, _, err = h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("", "$test1", 10),
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorTokenExpired, err)
}

func TestClientSubscribePermissionDeniedForAnonymous(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.SubscribeForClient = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)

	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, _, err = h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "test1",
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPublishNotAllowed(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "non_existing_namespace:test1",
		Data:    []byte(`{}`),
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPublishAllowed(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.PublishForClient = true
	ruleConfig.PublishForAnonymous = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil, proxy.PerCallData{})
	require.NoError(t, err)
}

func TestClientPublishForSubscriber(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.PublishForSubscriber = true
	ruleConfig.PublishForAnonymous = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil, proxy.PerCallData{})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientHistory(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.HistorySize = 10
	ruleConfig.HistoryTTL = tools.Duration(300 * time.Second)
	ruleConfig.HistoryForClient = true
	ruleConfig.HistoryForAnonymous = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()
	client.HandleCommand(&protocol.Command{Id: 1, Connect: &protocol.ConnectRequest{}}, 0)
	require.NoError(t, client.Subscribe("test1"))

	_, err = h.OnHistory(client, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.NoError(t, err)
}

func TestClientHistoryError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	ruleConfig = ruleContainer.Config()
	ruleConfig.HistorySize = 10
	ruleConfig.HistoryTTL = tools.Duration(300 * time.Second)
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPresence(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.Presence = true
	ruleConfig.PresenceForClient = true
	ruleConfig.PresenceForAnonymous = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()
	client.HandleCommand(&protocol.Command{Id: 1, Connect: &protocol.ConnectRequest{}}, 0)
	require.NoError(t, client.Subscribe("test1"))

	_, err = h.OnPresence(client, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.NoError(t, err)
}

func TestClientPresenceError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	ruleConfig = ruleContainer.Config()
	ruleConfig.Presence = true
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPresenceStats(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.Presence = true
	ruleConfig.PresenceForClient = true
	ruleConfig.PresenceForAnonymous = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: "secret",
	}, ruleContainer), &ProxyMap{}, false)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()
	client.HandleCommand(&protocol.Command{Id: 1, Connect: &protocol.ConnectRequest{}}, 0)
	require.NoError(t, client.Subscribe("test1"))

	_, err = h.OnPresenceStats(client, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.NoError(t, err)
}

func TestClientPresenceStatsError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	h := NewHandler(node, ruleContainer, jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}, ruleContainer), &ProxyMap{}, false)

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	ruleConfig = ruleContainer.Config()
	ruleConfig.Presence = true
	require.NoError(t, ruleContainer.Reload(ruleConfig))

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientOnSubscribe_UserLimitedChannelDoesNotCallProxy(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.UserLimitedChannels = true
	ruleConfig.ProxySubscribe = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)

	h := NewHandler(node, ruleContainer, nil, &ProxyMap{}, false)

	numProxyCalls := 0

	proxyFunc := func(c proxy.Client, e centrifuge.SubscribeEvent, chOpts rule.ChannelOptions, pcd proxy.PerCallData) (centrifuge.SubscribeReply, proxy.SubscribeExtra, error) {
		numProxyCalls++
		return centrifuge.SubscribeReply{}, proxy.SubscribeExtra{}, nil
	}

	_, _, err = h.OnSubscribe(tools.TestClientMock{
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user#42",
	}, proxyFunc, proxy.PerCallData{})
	require.NoError(t, err)
	require.Equal(t, 0, numProxyCalls)

	_, _, err = h.OnSubscribe(tools.TestClientMock{
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user",
	}, proxyFunc, proxy.PerCallData{})
	require.NoError(t, err)
	require.Equal(t, 1, numProxyCalls)
}

func TestClientOnSubscribe_UserLimitedChannelNotAllowedForAnotherUser(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig := rule.DefaultConfig
	ruleConfig.UserLimitedChannels = true
	ruleConfig.SubscribeForClient = true
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)

	h := NewHandler(node, ruleContainer, nil, &ProxyMap{}, false)

	numProxyCalls := 0

	proxyFunc := func(c proxy.Client, e centrifuge.SubscribeEvent, chOpts rule.ChannelOptions, pcd proxy.PerCallData) (centrifuge.SubscribeReply, proxy.SubscribeExtra, error) {
		numProxyCalls++
		return centrifuge.SubscribeReply{}, proxy.SubscribeExtra{}, nil
	}

	_, _, err = h.OnSubscribe(tools.TestClientMock{
		IDFunc: func() string {
			return "1"
		},
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user#42",
	}, proxyFunc, proxy.PerCallData{})
	require.NoError(t, err)

	_, _, err = h.OnSubscribe(tools.TestClientMock{
		IDFunc: func() string {
			return "2"
		},
		UserIDFunc: func() string {
			return "43"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user#42",
	}, proxyFunc, proxy.PerCallData{})
	require.ErrorIs(t, err, centrifuge.ErrorPermissionDenied)
}
