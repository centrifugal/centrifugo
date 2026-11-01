package client

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/proxy"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// subscribeSitesChannelOptions sets every option the subscribe option sites
// read.
func subscribeSitesChannelOptions() configtypes.ChannelOptions {
	return configtypes.ChannelOptions{
		Presence:                    true,
		JoinLeave:                   true,
		ForcePushJoinLeave:          true,
		HistorySize:                 10,
		HistoryTTL:                  configtypes.Duration(time.Minute),
		HistoryMetaTTL:              configtypes.Duration(time.Hour),
		ForceRecovery:               true,
		ForcePositioning:            true,
		ForceRecoveryMode:           "cache",
		AutoCacheRecover:            true,
		RecoveryMaxPublicationLimit: 7,
		AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
		AllowTagsFilter:             true,
		SubscribeForClient:          true,
	}
}

func subscribeSitesConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.Client.SubscribeToUserPersonalChannel.Enabled = true
	cfg.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace = "ns"
	cfg.Channel.Proxy.Subscribe.Endpoint = "http://localhost:8080"
	cfg.Channel.Proxy.SubscribeStream.Endpoint = "grpc://localhost:12000"

	subscribeProxyOpts := subscribeSitesChannelOptions()
	subscribeProxyOpts.SubscribeProxyEnabled = true
	subscribeProxyOpts.SubscribeProxyName = "default"

	streamProxyOpts := subscribeSitesChannelOptions()
	streamProxyOpts.SubscribeStreamProxyEnabled = true
	streamProxyOpts.SubscribeStreamProxyName = "default"

	mapOpts := subscribeSitesChannelOptions()
	mapOpts.SubscriptionType = "map"
	mapOpts.Map.Mode = "ephemeral"
	mapOpts.Map.KeyTTL = configtypes.Duration(time.Minute)
	mapOpts.Map.RemoveClientOnUnsubscribe = true
	mapOpts.MapClientsPresenceChannelPrefix = "mc:"
	mapOpts.MapUsersPresenceChannelPrefix = "mu:"

	mapClientsOpts := configtypes.ChannelOptions{SubscriptionType: "map_clients"}
	mapClientsOpts.Map.Mode = "ephemeral"
	mapClientsOpts.Map.KeyTTL = configtypes.Duration(time.Minute)
	mapUsersOpts := configtypes.ChannelOptions{SubscriptionType: "map_users"}
	mapUsersOpts.Map.Mode = "ephemeral"
	mapUsersOpts.Map.KeyTTL = configtypes.Duration(time.Minute)

	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{Name: "ns", ChannelOptions: subscribeSitesChannelOptions()},
		{Name: "sp", ChannelOptions: subscribeProxyOpts},
		{Name: "ssp", ChannelOptions: streamProxyOpts},
		{Name: "nm", ChannelOptions: mapOpts},
		{Name: "mc", ChannelOptions: mapClientsOpts},
		{Name: "mu", ChannelOptions: mapUsersOpts},
	}
	return cfg
}

// TestSubscribeOptionSites pins the subscribe options built from channel
// options at the client handler sites: the connect reply subscriptions and the
// final OnSubscribe reply of every path.
func TestSubscribeOptionSites(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfgContainer, err := config.NewContainer(subscribeSitesConfig())
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, hmacJWTVerifier(t, cfgContainer), nil, &ProxyMap{})

	client := &tools.TestClientMock{
		IDFunc:     func() string { return "client" },
		UserIDFunc: func() string { return "42" },
	}

	t.Run("connect", func(t *testing.T) {
		reply, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
			Token:    getConnTokenHS("42", 0),
			Channels: []string{"ns:a"},
		}, nil, false)
		require.NoError(t, err)
		expected := centrifuge.SubscribeOptions{
			EmitPresence:                true,
			EmitJoinLeave:               true,
			PushJoinLeave:               true,
			EnableRecovery:              true,
			EnablePositioning:           true,
			RecoveryMode:                centrifuge.RecoveryModeCache,
			AutoCacheRecover:            true,
			HistoryMetaTTL:              time.Hour,
			RecoveryMaxPublicationLimit: 7,
			AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
			AllowTagsFilter:             true,
		}
		personal := expected
		personal.Source = subsource.UserPersonal
		connectChannel := expected
		connectChannel.Source = subsource.UniConnect
		require.Equal(t, map[string]centrifuge.SubscribeOptions{
			cfgContainer.PersonalChannel("42"): personal,
			"ns:a":                             connectChannel,
		}, reply.Subscriptions)
	})

	streamReply := centrifuge.SubscribeOptions{
		EmitPresence:                true,
		EmitJoinLeave:               true,
		PushJoinLeave:               true,
		EnableRecovery:              true,
		EnablePositioning:           true,
		RecoveryMode:                centrifuge.RecoveryModeCache,
		AutoCacheRecover:            true,
		HistoryMetaTTL:              time.Hour,
		RecoveryMaxPublicationLimit: 7,
		AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
		AllowTagsFilter:             true,
	}

	t.Run("client_subscribe", func(t *testing.T) {
		reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{Channel: "ns:s"}, nil, nil)
		require.NoError(t, err)
		expected := streamReply
		expected.Source = subsource.ClientAllowed
		require.Equal(t, centrifuge.SubscribeReply{
			Options:           expected,
			ClientSideRefresh: true,
		}, reply)
	})

	t.Run("client_subscribe_map", func(t *testing.T) {
		reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
			Channel: "nm:s",
			Type:    centrifuge.SubscriptionTypeMap,
		}, nil, nil)
		require.NoError(t, err)
		expected := streamReply
		expected.Source = subsource.ClientAllowed
		expected.Type = centrifuge.SubscriptionTypeMap
		expected.MapRemoveClientOnUnsubscribe = true
		expected.MapClientPresenceChannel = "mc:nm:s"
		expected.MapUserPresenceChannel = "mu:nm:s"
		require.Equal(t, centrifuge.SubscribeReply{
			Options:           expected,
			ClientSideRefresh: true,
		}, reply)
	})

	t.Run("subscription_token", func(t *testing.T) {
		builder := getTokenBuilder(nil, "secret")
		token, err := builder.Build(&jwtverify.SubscribeTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "42"},
			Channel:          "ns:t",
			SubscribeOptions: jwtverify.SubscribeOptions{
				Base64Info: "e30=",
				Data:       json.RawMessage(`{"d":1}`),
				Override: &jwtverify.SubscribeOptionOverride{
					ForceRecovery: &jwtverify.BoolValue{Value: false},
				},
			},
		})
		require.NoError(t, err)
		reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{
			Channel: "ns:t",
			Token:   token.String(),
		}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ChannelInfo:                 []byte("{}"),
				EmitPresence:                true,
				EmitJoinLeave:               true,
				PushJoinLeave:               true,
				EnableRecovery:              false,
				EnablePositioning:           true,
				RecoveryMode:                centrifuge.RecoveryModeCache,
				Data:                        []byte(`{"d":1}`),
				Source:                      subsource.SubscriptionToken,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
				AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
				AllowTagsFilter:             true,
			},
			ClientSideRefresh: true,
		}, reply)
	})

	t.Run("subscribe_proxy", func(t *testing.T) {
		proxyFunc := func(c proxy.Client, e centrifuge.SubscribeEvent, chOpts configtypes.ChannelOptions, pcd proxy.PerCallData) (centrifuge.SubscribeReply, proxy.SubscribeExtra, error) {
			return centrifuge.SubscribeReply{
				Options: centrifuge.SubscribeOptions{
					EmitPresence:   true,
					EmitJoinLeave:  true,
					EnableRecovery: true,
					RecoveryMode:   centrifuge.RecoveryModeCache,
					Source:         subsource.SubscribeProxy,
				},
				ClientSideRefresh: true,
			}, proxy.SubscribeExtra{}, nil
		}
		reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{Channel: "sp:s"}, proxyFunc, nil)
		require.NoError(t, err)
		require.Equal(t, centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				EmitPresence:                true,
				EmitJoinLeave:               true,
				EnableRecovery:              true,
				RecoveryMode:                centrifuge.RecoveryModeCache,
				AutoCacheRecover:            true,
				Source:                      subsource.SubscribeProxy,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
			},
			ClientSideRefresh: true,
		}, reply)
	})

	t.Run("subscribe_stream_proxy", func(t *testing.T) {
		streamFunc := func(c proxy.Client, bidi bool, e centrifuge.SubscribeEvent, chOpts configtypes.ChannelOptions, pcd proxy.PerCallData) (centrifuge.SubscribeReply, proxy.StreamPublishFunc, func(), error) {
			return centrifuge.SubscribeReply{
				Options: centrifuge.SubscribeOptions{
					EmitPresence:  true,
					EmitJoinLeave: true,
					Source:        subsource.StreamProxy,
				},
				ClientSideRefresh: true,
			}, nil, nil, nil
		}
		reply, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{Channel: "ssp:s"}, nil, streamFunc)
		require.NoError(t, err)
		require.Equal(t, centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				EmitPresence:                true,
				EmitJoinLeave:               true,
				Source:                      subsource.StreamProxy,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
			},
			ClientSideRefresh: true,
		}, reply)
	})
}

func BenchmarkSubscribeOptionSites(b *testing.B) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfgContainer, err := config.NewContainer(subscribeSitesConfig())
	require.NoError(b, err)
	verifier, err := jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{HMACSecretKey: "secret"}, cfgContainer)
	require.NoError(b, err)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

	client := &tools.TestClientMock{
		IDFunc:     func() string { return "client" },
		UserIDFunc: func() string { return "42" },
	}

	connectToken := getConnTokenHS("42", 0)
	subscribeToken := getSubscribeTokenHS("42", "ns:t", 0)

	b.Run("connect", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
				Token:    connectToken,
				Channels: []string{"ns:a"},
			}, nil, false)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("client_subscribe", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{Channel: "ns:s"}, nil, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("subscription_token", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, err := h.OnSubscribe(client, centrifuge.SubscribeEvent{Channel: "ns:t", Token: subscribeToken}, nil, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
}
