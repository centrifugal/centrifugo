package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
	"github.com/centrifugal/centrifugo/v6/internal/subsource"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// subscribeSitesChannelOptions sets every option the subscribe option sites
// read.
func subscribeSitesChannelOptions() configtypes.ChannelOptions {
	return configtypes.ChannelOptions{
		Presence:                     true,
		JoinLeave:                    true,
		ForcePushJoinLeave:           true,
		HistorySize:                  10,
		HistoryTTL:                   configtypes.Duration(time.Minute),
		HistoryMetaTTL:               configtypes.Duration(time.Hour),
		ForceRecovery:                true,
		ForcePositioning:             true,
		ForceRecoveryMode:            "cache",
		AutoCacheRecover:             true,
		RecoveryMaxPublicationLimit:  7,
		AllowedDeltaTypes:            []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
		AllowTagsFilter:              true,
		SubscribeProxyEnabled:        true,
		SubscribeProxyName:           "test",
		SubscribeStreamProxyEnabled:  true,
		SubscribeStreamProxyName:     "test",
		SubscribeStreamBidirectional: false,
	}
}

type subscribeSitesProxyServer struct {
	proxyproto.UnimplementedCentrifugoProxyServer
}

func (subscribeSitesProxyServer) Connect(_ context.Context, _ *proxyproto.ConnectRequest) (*proxyproto.ConnectResponse, error) {
	return &proxyproto.ConnectResponse{
		Result: &proxyproto.ConnectResult{
			User:     "42",
			Channels: []string{"ns:a"},
			Subs: map[string]*proxyproto.SubscribeOptions{
				"ns:b": {
					B64Info:      "e30=",
					Data:         proxyproto.Raw(`{"d":1}`),
					CacheRecover: true,
					Override: &proxyproto.SubscribeOptionOverride{
						Presence:         &proxyproto.BoolValue{Value: false},
						ForcePositioning: &proxyproto.BoolValue{Value: false},
					},
				},
			},
		},
	}, nil
}

func (subscribeSitesProxyServer) Subscribe(_ context.Context, _ *proxyproto.SubscribeRequest) (*proxyproto.SubscribeResponse, error) {
	return &proxyproto.SubscribeResponse{
		Result: &proxyproto.SubscribeResult{
			ExpireAt: 100,
			B64Info:  "e30=",
			Data:     proxyproto.Raw(`{"d":2}`),
			Override: &proxyproto.SubscribeOptionOverride{
				JoinLeave: &proxyproto.BoolValue{Value: false},
			},
		},
	}, nil
}

func (subscribeSitesProxyServer) SubscribeUnidirectional(_ *proxyproto.SubscribeRequest, stream grpc.ServerStreamingServer[proxyproto.StreamSubscribeResponse]) error {
	err := stream.Send(&proxyproto.StreamSubscribeResponse{
		SubscribeResponse: &proxyproto.SubscribeResponse{
			Result: &proxyproto.SubscribeResult{
				ExpireAt: 200,
				Info:     proxyproto.Raw(`{"i":1}`),
				Data:     proxyproto.Raw(`{"d":3}`),
				Override: &proxyproto.SubscribeOptionOverride{
					Presence: &proxyproto.BoolValue{Value: false},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

// TestSubscribeOptionSites pins the subscribe options built from channel
// options at the proxy sites.
func TestSubscribeOptionSites(t *testing.T) {
	commonProxyTestCase := tools.NewCommonGRPCProxyTestCase(context.Background(), subscribeSitesProxyServer{})
	defer commonProxyTestCase.Teardown()

	t.Run("connect_proxy", func(t *testing.T) {
		chOpts := subscribeSitesChannelOptions()
		chOpts.SubscribeProxyEnabled = false
		chOpts.SubscribeStreamProxyEnabled = false
		chOpts.SubscribeProxyName = ""
		chOpts.SubscribeStreamProxyName = ""
		cfg := config.DefaultConfig()
		cfg.Channel.Namespaces = []configtypes.ChannelNamespace{{
			Name:           "ns",
			ChannelOptions: chOpts,
		}}
		cfgContainer, err := config.NewContainer(cfg)
		require.NoError(t, err)
		connectProxy, err := NewGRPCConnectProxy("default", getTestGrpcProxy(commonProxyTestCase))
		require.NoError(t, err)
		h := NewConnectHandler(ConnectHandlerConfig{Proxy: connectProxy}, cfgContainer)

		reply, _, err := h.Handle()(context.Background(), centrifuge.ConnectEvent{
			Transport: tools.NewTestTransport(),
		})
		require.NoError(t, err)
		require.Equal(t, map[string]centrifuge.SubscribeOptions{
			// Connect result channels.
			"ns:a": {
				EmitPresence:                true,
				EmitJoinLeave:               true,
				PushJoinLeave:               true,
				EnableRecovery:              true,
				EnablePositioning:           true,
				RecoveryMode:                centrifuge.RecoveryModeCache,
				Source:                      subsource.ConnectProxy,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
				AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
				AllowTagsFilter:             true,
			},
			// Connect result subs.
			"ns:b": {
				ChannelInfo:                 []byte("{}"),
				EmitPresence:                false,
				EmitJoinLeave:               true,
				PushJoinLeave:               true,
				EnableRecovery:              true,
				EnablePositioning:           false,
				RecoveryMode:                centrifuge.RecoveryModeCache,
				AutoCacheRecover:            true,
				Data:                        []byte(`{"d":1}`),
				Source:                      subsource.ConnectProxy,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
				AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
				AllowTagsFilter:             true,
			},
		}, reply.Subscriptions)
	})

	t.Run("subscribe_proxy", func(t *testing.T) {
		subscribeProxy, err := NewGRPCSubscribeProxy("test", getTestGrpcProxy(commonProxyTestCase))
		require.NoError(t, err)
		h := NewSubscribeHandler(SubscribeHandlerConfig{
			Proxies: map[string]SubscribeProxy{"test": subscribeProxy},
		})
		reply, _, err := h.Handle()(commonProxyTestCase.Client, centrifuge.SubscribeEvent{Channel: "ns:c"}, subscribeSitesChannelOptions(), PerCallData{})
		require.NoError(t, err)
		require.Equal(t, centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ExpireAt:                    100,
				ChannelInfo:                 []byte("{}"),
				EmitPresence:                true,
				EmitJoinLeave:               false,
				PushJoinLeave:               true,
				EnableRecovery:              true,
				EnablePositioning:           true,
				RecoveryMode:                centrifuge.RecoveryModeCache,
				AutoCacheRecover:            true,
				AllowedDeltaTypes:           []centrifuge.DeltaType{centrifuge.DeltaTypeFossil},
				AllowTagsFilter:             true,
				Data:                        []byte(`{"d":2}`),
				Source:                      subsource.SubscribeProxy,
				HistoryMetaTTL:              time.Hour,
				RecoveryMaxPublicationLimit: 7,
			},
			ClientSideRefresh: true,
		}, reply)
	})

	t.Run("subscribe_stream_proxy", func(t *testing.T) {
		streamProxy, err := NewSubscribeStreamProxy("test", getTestGrpcProxy(commonProxyTestCase))
		require.NoError(t, err)
		h := NewSubscribeStreamHandler(SubscribeStreamHandlerConfig{
			Proxies: map[string]*SubscribeStreamProxy{"test": streamProxy},
		})
		reply, _, cancel, err := h.Handle()(commonProxyTestCase.Client, false, centrifuge.SubscribeEvent{Channel: "ns:d"}, subscribeSitesChannelOptions(), PerCallData{})
		require.NoError(t, err)
		require.NotNil(t, cancel)
		defer cancel()
		require.NotNil(t, reply.SubscriptionReady)
		reply.SubscriptionReady = nil
		require.Equal(t, centrifuge.SubscribeReply{
			Options: centrifuge.SubscribeOptions{
				ExpireAt:      200,
				ChannelInfo:   []byte(`{"i":1}`),
				EmitPresence:  false,
				EmitJoinLeave: true,
				PushJoinLeave: true,
				Data:          []byte(`{"d":3}`),
				Source:        subsource.StreamProxy,
			},
			ClientSideRefresh: true,
		}, reply)
	})
}
