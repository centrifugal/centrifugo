package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/proxy"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

func generateTestRSAKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	reader := rand.Reader
	bitSize := 2048
	key, err := rsa.GenerateKey(reader, bitSize)
	require.NoError(t, err)
	return key, &key.PublicKey
}

func getTokenBuilder(rsaPrivateKey *rsa.PrivateKey, hmacSecret string) *jwt.Builder {
	var signer jwt.Signer
	if rsaPrivateKey != nil {
		signer, _ = jwt.NewSignerRS(jwt.RS256, rsaPrivateKey)
	} else {
		// For HS we do everything in tests with key `secret`.
		key := []byte(hmacSecret)
		signer, _ = jwt.NewSignerHS(jwt.HS256, key)
	}
	return jwt.NewBuilder(signer)
}

func getConnToken(user string, exp int64, rsaPrivateKey *rsa.PrivateKey) string {
	builder := getTokenBuilder(rsaPrivateKey, "secret")
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

func getSubscribeToken(user string, channel string, exp int64, rsaPrivateKey *rsa.PrivateKey, hmacSecret string) string {
	builder := getTokenBuilder(rsaPrivateKey, hmacSecret)
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
	return getSubscribeTokenHSWithSecret(user, channel, exp, "secret")
}

func getSubscribeTokenHSWithSecret(user string, channel string, exp int64, hmacSecret string) string {
	return getSubscribeToken(user, channel, exp, nil, hmacSecret)
}

func emptyJWTVerifier(t *testing.T, cfgContainer *config.Container) *jwtverify.VerifierJWT {
	verifier, err := jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{}, cfgContainer)
	require.NoError(t, err)
	return verifier
}

func hmacJWTVerifier(t *testing.T, cfgContainer *config.Container) *jwtverify.VerifierJWT {
	return hmacJWTVerifierWithSecret(t, cfgContainer, "secret")
}

func hmacJWTVerifierWithSecret(t *testing.T, cfgContainer *config.Container, secret string) *jwtverify.VerifierJWT {
	verifier, err := jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		HMACSecretKey: secret,
	}, cfgContainer)
	require.NoError(t, err)
	return verifier
}

func TestClientHandlerSetup(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.Insecure = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, emptyJWTVerifier(t, cfgContainer), nil, &ProxyMap{})
	err = h.Setup()
	require.NoError(t, err)

	transport := tools.NewTestTransport()
	client, closeFn, err := centrifuge.NewClient(context.Background(), node, transport)
	require.NoError(t, err)
	defer func() { _ = closeFn() }()

	connectCommand := &protocol.Command{
		Id:      1,
		Connect: &protocol.ConnectRequest{},
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

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, emptyJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Transport: tools.NewTestTransport(),
	}, nil, false)
	require.NoError(t, err)
	require.Nil(t, reply.Credentials)
}

func TestClientConnectingNoCredentialsNoTokenInsecure(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.Insecure = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, emptyJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "", reply.Credentials.UserID)
}

func TestClientConnectNoCredentialsNoTokenAnonymous(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.AllowAnonymousConnectWithoutToken = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, emptyJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{}, nil, false)
	require.NoError(t, err)

	require.NotNil(t, reply.Credentials)
	require.Equal(t, "", reply.Credentials.UserID)
}

func TestClientConnectWithMalformedToken(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier := hmacJWTVerifier(t, cfgContainer)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

	_, err = h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: "bad bad token",
	}, nil, false)
	require.Error(t, err)
}

func TestClientConnectWithValidTokenHMAC(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier := hmacJWTVerifier(t, cfgContainer)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

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

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier := hmacJWTVerifier(t, cfgContainer)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

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

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{
		RSAPublicKey: pubKey,
	}, cfgContainer)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

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

	cfg := config.DefaultConfig()
	cfg.Client.AllowAnonymousConnectWithoutToken = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier := hmacJWTVerifier(t, cfgContainer)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

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

	cfg := config.DefaultConfig()
	cfg.Client.AllowAnonymousConnectWithoutToken = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier := hmacJWTVerifier(t, cfgContainer)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

	_, err = h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
		Token: getConnTokenHS("42", 1525541722),
	}, nil, false)
	require.Equal(t, centrifuge.ErrorTokenExpired, err)
}

func TestClientSideRefresh(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.AllowAnonymousConnectWithoutToken = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier := hmacJWTVerifier(t, cfgContainer)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

	reply, _, err := h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("", 123),
	}, nil)
	require.NoError(t, err)
	require.True(t, reply.Expired)

	_, _, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: "invalid",
	}, nil)
	require.Error(t, err)

	reply, _, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("", 2525637058),
	}, nil)
	require.NoError(t, err)
	require.False(t, reply.Expired)
	require.Equal(t, int64(2525637058), reply.ExpireAt)
}

func TestClientSideRefreshDifferentUser(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.AllowAnonymousConnectWithoutToken = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier := hmacJWTVerifier(t, cfgContainer)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

	_, _, err = h.OnRefresh(&centrifuge.Client{}, centrifuge.RefreshEvent{
		Token: getConnTokenHS("42", 2525637058),
	}, nil)
	require.ErrorIs(t, err, centrifuge.DisconnectInvalidToken)
}

func TestClientUserPersonalChannel(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.SubscribeToUserPersonalChannel.Enabled = true
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name:           "user",
			ChannelOptions: configtypes.ChannelOptions{},
		},
	}
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	var tests = []struct {
		Name      string
		Namespace string
	}{
		{"ok_no_namespace", ""},
		{"ok_with_namespace", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			cfg := cfgContainer.Config()
			cfg.Client.SubscribeToUserPersonalChannel.Enabled = true
			cfg.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace = tt.Namespace
			err := cfgContainer.Reload(cfg)
			require.NoError(t, err)
			reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
				Token: getConnTokenHS("42", 0),
			}, nil, false)
			require.NoError(t, err)
			require.Contains(t, reply.Subscriptions, cfgContainer.PersonalChannel("42"))
		})
	}
}

func TestClientSubscribeChannel(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "non_existing_namespace:test1",
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)
}

func TestClientSubscribeChannelNoPermission(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test1",
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSubscribeChannelUserLimitedError(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.UserLimitedChannels = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test#13",
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientSubscribeChannelUserLimitedOK(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.UserLimitedChannels = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "test#12",
	}, nil, nil)
	require.NoError(t, err)
}

func TestClientSubscribeWithToken(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "",
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   "invalid",
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", 123),
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorTokenExpired, err)

	reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", 0),
	}, nil, nil)
	require.NoError(t, err)
	require.Zero(t, reply.Options.ExpireAt)
}

func TestClientSubscribeWithTokenAnonymous(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("", "$test1", 0),
	}, nil, nil)
	require.NoError(t, err)
}

func TestClientSideSubRefresh(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.AllowAnonymousConnectWithoutToken = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", time.Now().Unix()+10),
	}, nil, nil)
	require.NoError(t, err)
	require.True(t, reply.Options.ExpireAt > 0)

	subRefreshReply, _, err := h.OnSubRefresh(client, nil, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("12", "$test1", 123),
	})
	require.NoError(t, err)
	require.True(t, subRefreshReply.Expired)

	subRefreshReply, _, err = h.OnSubRefresh(client, nil, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   "invalid",
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, err)

	subRefreshReply, _, err = h.OnSubRefresh(client, nil, centrifuge.SubRefreshEvent{
		Channel: "$test2",
		Token:   getSubscribeTokenHS("12", "$test1", 2525637058),
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, err)

	subRefreshReply, _, err = h.OnSubRefresh(client, nil, centrifuge.SubRefreshEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", 2525637058),
	})
	require.NoError(t, err)
	require.False(t, subRefreshReply.Expired)
	require.Equal(t, int64(2525637058), subRefreshReply.ExpireAt)
}

func TestClientSideSubRefresh_SeparateSubTokenConfig(t *testing.T) {
	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.AllowAnonymousConnectWithoutToken = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), hmacJWTVerifierWithSecret(t, cfgContainer, "new_secret"), &ProxyMap{})

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
		Id:      1,
		Connect: &protocol.ConnectRequest{},
	}
	encoder := protocol.NewJSONCommandEncoder()
	data, err := encoder.Encode(connectCommand)
	require.NoError(t, err)
	ok := centrifuge.HandleReadFrame(client, bytes.NewReader(data))
	require.True(t, ok)

	_, _, err = h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", time.Now().Unix()+10),
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)

	reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHSWithSecret("12", "$test1", time.Now().Unix()+10, "new_secret"),
	}, nil, nil)
	require.NoError(t, err)
	require.True(t, reply.Options.ExpireAt > 0)

	subRefreshReply, _, err := h.OnSubRefresh(client, nil, centrifuge.SubRefreshEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHSWithSecret("12", "$test1", 2525637058, "new_secret"),
	})
	require.NoError(t, err)
	require.False(t, subRefreshReply.Expired)
	require.Equal(t, int64(2525637058), subRefreshReply.ExpireAt)

	_, _, err = h.OnSubRefresh(client, nil, centrifuge.SubRefreshEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("12", "$test1", 2525637058),
	})
	require.Equal(t, centrifuge.DisconnectInvalidToken, err)
}

func TestClientSubscribePrivateChannelWithExpiringToken(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, _, err = h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "$test1",
		Token:   getSubscribeTokenHS("", "$test1", 10),
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorTokenExpired, err)
}

func TestClientSubscribePermissionDeniedForAnonymous(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.SubscribeForClient = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, _, err = h.OnSubscribe(&centrifuge.Client{}, centrifuge.SubscribeEvent{
		Channel: "test1",
	}, nil, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPublishNotAllowed(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
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
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.PublishForClient = true
	cfg.Channel.WithoutNamespace.PublishForAnonymous = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil)
	require.NoError(t, err)
}

func TestClientPublishForSubscriber(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.PublishForSubscriber = true
	cfg.Channel.WithoutNamespace.PublishForAnonymous = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, err = h.OnPublish(&centrifuge.Client{}, centrifuge.PublishEvent{
		Channel: "test1",
		Data:    []byte(`{}`),
	}, nil)
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientHistory(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}, nil
	})

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.HistorySize = 10
	cfg.Channel.WithoutNamespace.HistoryTTL = configtypes.Duration(300 * time.Second)
	cfg.Channel.WithoutNamespace.HistoryForClient = true
	cfg.Channel.WithoutNamespace.HistoryForAnonymous = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	cfg = cfgContainer.Config()
	cfg.Channel.WithoutNamespace.HistorySize = 10
	cfg.Channel.WithoutNamespace.HistoryTTL = configtypes.Duration(300 * time.Second)
	require.NoError(t, cfgContainer.Reload(cfg))

	_, err = h.OnHistory(&centrifuge.Client{}, centrifuge.HistoryEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPresence(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}, nil
	})

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.Presence = true
	cfg.Channel.WithoutNamespace.PresenceForClient = true
	cfg.Channel.WithoutNamespace.PresenceForAnonymous = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, emptyJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	cfg = cfgContainer.Config()
	cfg.Channel.WithoutNamespace.Presence = true
	require.NoError(t, cfgContainer.Reload(cfg))

	_, err = h.OnPresence(&centrifuge.Client{}, centrifuge.PresenceEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientPresenceStats(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{
				UserID: "",
			},
		}, nil
	})

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.Presence = true
	cfg.Channel.WithoutNamespace.PresenceForClient = true
	cfg.Channel.WithoutNamespace.PresenceForAnonymous = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

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

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, emptyJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "non_existing_namespace:test1",
	})
	require.Equal(t, centrifuge.ErrorUnknownChannel, err)

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorNotAvailable, err)

	cfg = cfgContainer.Config()
	cfg.Channel.WithoutNamespace.Presence = true
	require.NoError(t, cfgContainer.Reload(cfg))

	_, err = h.OnPresenceStats(&centrifuge.Client{}, centrifuge.PresenceStatsEvent{
		Channel: "test1",
	})
	require.Equal(t, centrifuge.ErrorPermissionDenied, err)
}

func TestClientOnSubscribe_UserLimitedChannelDoesNotCallProxy(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.Proxy.Subscribe.Endpoint = "http://localhost:8080"
	cfg.Channel.WithoutNamespace.UserLimitedChannels = true
	cfg.Channel.WithoutNamespace.SubscribeProxyEnabled = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	h := NewHandler(node, cfgContainer, nil, nil, &ProxyMap{})

	numProxyCalls := 0

	proxyFunc := func(c proxy.Client, e centrifuge.SubscribeEvent, chOpts configtypes.ChannelOptions, pcd proxy.PerCallData) (centrifuge.SubscribeReply, proxy.SubscribeExtra, error) {
		numProxyCalls++
		return centrifuge.SubscribeReply{}, proxy.SubscribeExtra{}, nil
	}

	_, _, err = h.OnSubscribe(&tools.TestClientMock{
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user#42",
	}, proxyFunc, nil)
	require.NoError(t, err)
	require.Equal(t, 0, numProxyCalls)

	_, _, err = h.OnSubscribe(&tools.TestClientMock{
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user",
	}, proxyFunc, nil)
	require.NoError(t, err)
	require.Equal(t, 1, numProxyCalls)
}

func TestClientOnSubscribe_UserLimitedChannelNotAllowedForAnotherUser(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.UserLimitedChannels = true
	cfg.Channel.WithoutNamespace.SubscribeForClient = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	h := NewHandler(node, cfgContainer, nil, nil, &ProxyMap{})

	numProxyCalls := 0

	proxyFunc := func(c proxy.Client, e centrifuge.SubscribeEvent, chOpts configtypes.ChannelOptions, pcd proxy.PerCallData) (centrifuge.SubscribeReply, proxy.SubscribeExtra, error) {
		numProxyCalls++
		return centrifuge.SubscribeReply{}, proxy.SubscribeExtra{}, nil
	}

	_, _, err = h.OnSubscribe(&tools.TestClientMock{
		IDFunc: func() string {
			return "1"
		},
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user#42",
	}, proxyFunc, nil)
	require.NoError(t, err)

	_, _, err = h.OnSubscribe(&tools.TestClientMock{
		IDFunc: func() string {
			return "2"
		},
		UserIDFunc: func() string {
			return "43"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user#42",
	}, proxyFunc, nil)
	require.ErrorIs(t, err, centrifuge.ErrorPermissionDenied)
}

func TestClientOnSubscribe_SubRefreshProxy(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.Proxy.Subscribe.Endpoint = "http://localhost:8080"
	cfg.Channel.WithoutNamespace.SubscribeProxyEnabled = true
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	h := NewHandler(node, cfgContainer, nil, nil, &ProxyMap{})

	numProxyCalls := 0

	proxyFunc := func(c proxy.Client, e centrifuge.SubscribeEvent, chOpts configtypes.ChannelOptions, pcd proxy.PerCallData) (centrifuge.SubscribeReply, proxy.SubscribeExtra, error) {
		numProxyCalls++
		return centrifuge.SubscribeReply{
			ClientSideRefresh: !chOpts.SubRefreshProxyEnabled,
		}, proxy.SubscribeExtra{}, nil
	}

	reply, _, err := h.OnSubscribe(&tools.TestClientMock{
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user",
	}, proxyFunc, nil)
	require.NoError(t, err)
	require.Equal(t, 1, numProxyCalls)
	require.True(t, reply.ClientSideRefresh)

	cfg.Channel.WithoutNamespace.SubRefreshProxyEnabled = true
	cfg.Channel.Proxy.SubRefresh.Endpoint = "https://example.com"
	err = cfgContainer.Reload(cfg)
	require.NoError(t, err)
	reply, _, err = h.OnSubscribe(&tools.TestClientMock{
		UserIDFunc: func() string {
			return "42"
		},
	}, centrifuge.SubscribeEvent{
		Channel: "user",
	}, proxyFunc, nil)
	require.NoError(t, err)
	require.Equal(t, 2, numProxyCalls)
	require.False(t, reply.ClientSideRefresh)
}
