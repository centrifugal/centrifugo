package api

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/metrics"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Initialize metrics with a custom registry for tests to avoid conflicts
	registry := prometheus.NewRegistry()
	_ = metrics.Init(metrics.Config{
		Registerer: registry,
	})
	os.Exit(m.Run())
}

func nodeWithMemoryEngine() *centrifuge.Node {
	n, err := centrifuge.New(centrifuge.Config{})
	if err != nil {
		panic(err)
	}
	err = n.Run()
	if err != nil {
		panic(err)
	}
	return n
}

type testSurveyCaller struct{}

func (t testSurveyCaller) Channels(_ context.Context, _ *ChannelsRequest) (map[string]*ChannelInfo, error) {
	return nil, nil
}

func (t testSurveyCaller) Connections(_ context.Context, _ *ConnectionsRequest) (map[string]*ConnectionInfo, error) {
	return nil, nil
}

func TestPublishAPI(t *testing.T) {
	node := nodeWithMemoryEngine()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Publish(context.Background(), &PublishRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{Channel: "test"})
	require.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{Channel: "test", Data: []byte("test")})
	require.Nil(t, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{Channel: "test:test", Data: []byte("test")})
	require.Equal(t, ErrorUnknownChannel, resp.Error)
}

func TestBroadcastAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Broadcast(context.Background(), &BroadcastRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)

	// Empty data validation now happens per-channel
	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test"}})
	require.Nil(t, resp.Error)
	require.Equal(t, ErrorBadRequest.Code, resp.Result.Responses[0].Error.Code)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test"}, Data: []byte("test")})
	require.Nil(t, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test:test"}, Data: []byte("test")})
	require.Nil(t, resp.Error)
	require.Equal(t, ErrorUnknownChannel, resp.Result.Responses[0].Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test", "test:test"}, Data: []byte("test")})
	require.Nil(t, resp.Error)
	require.Equal(t, ErrorUnknownChannel, resp.Result.Responses[1].Error)
}

func TestPublishAPI_DataFormat_JSON(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.PublicationDataFormat = configtypes.PublicationDataFormatJSON
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	// Valid JSON should succeed
	resp := api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte(`{"key": "value"}`),
	})
	require.Nil(t, resp.Error)

	// Valid JSON array should succeed
	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte(`[1, 2, 3]`),
	})
	require.Nil(t, resp.Error)

	// Valid JSON primitives should succeed
	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte(`"string"`),
	})
	require.Nil(t, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte(`123`),
	})
	require.Nil(t, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte(`true`),
	})
	require.Nil(t, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte(`null`),
	})
	require.Nil(t, resp.Error)

	// Invalid JSON should fail
	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte(`not valid json`),
	})
	require.NotNil(t, resp.Error)
	require.Equal(t, ErrorBadRequest.Code, resp.Error.Code)

	// Empty data with JSON format should fail
	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte{},
	})
	require.NotNil(t, resp.Error)
	require.Equal(t, ErrorBadRequest.Code, resp.Error.Code)
}

func TestPublishAPI_DataFormat_Binary(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.PublicationDataFormat = configtypes.PublicationDataFormatBinary
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	// Binary data should succeed
	resp := api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte{0x01, 0x02, 0x03},
	})
	require.Nil(t, resp.Error)

	// Empty data with binary format should succeed
	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte{},
	})
	require.Nil(t, resp.Error)

	// Any data should succeed with binary format
	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte("any text data"),
	})
	require.Nil(t, resp.Error)
}

func TestPublishAPI_DataFormat_Default(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	// Default format (empty string) - should reject empty data
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	// Non-empty data should succeed
	resp := api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte("any data"),
	})
	require.Nil(t, resp.Error)

	// Empty data should fail (current behavior preserved)
	resp = api.Publish(context.Background(), &PublishRequest{
		Channel: "test",
		Data:    []byte{},
	})
	require.NotNil(t, resp.Error)
	require.Equal(t, ErrorBadRequest.Code, resp.Error.Code)
}

func TestBroadcastAPI_DataFormat_JSON(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.PublicationDataFormat = configtypes.PublicationDataFormatJSON
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	// Valid JSON should succeed
	resp := api.Broadcast(context.Background(), &BroadcastRequest{
		Channels: []string{"test", "test2"},
		Data:     []byte(`{"key": "value"}`),
	})
	require.Nil(t, resp.Error)
	require.Nil(t, resp.Result.Responses[0].Error)
	require.Nil(t, resp.Result.Responses[1].Error)

	// Invalid JSON should fail for all channels
	resp = api.Broadcast(context.Background(), &BroadcastRequest{
		Channels: []string{"test", "test2"},
		Data:     []byte(`not valid json`),
	})
	require.Nil(t, resp.Error)
	require.Equal(t, ErrorBadRequest.Code, resp.Result.Responses[0].Error.Code)
	require.Equal(t, ErrorBadRequest.Code, resp.Result.Responses[1].Error.Code)
}

func TestBroadcastAPI_DataFormat_Binary(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Channel.WithoutNamespace.PublicationDataFormat = configtypes.PublicationDataFormatBinary
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	// Empty data should succeed with binary format
	resp := api.Broadcast(context.Background(), &BroadcastRequest{
		Channels: []string{"test", "test2"},
		Data:     []byte{},
	})
	require.Nil(t, resp.Error)
	require.Nil(t, resp.Result.Responses[0].Error)
	require.Nil(t, resp.Result.Responses[1].Error)
}

func TestHistoryAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.History(context.Background(), &HistoryRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.History(context.Background(), &HistoryRequest{Channel: "test"})
	require.Equal(t, ErrorNotAvailable, resp.Error)

	cfg = cfgContainer.Config()
	cfg.Channel.WithoutNamespace.HistorySize = 1
	cfg.Channel.WithoutNamespace.HistoryTTL = configtypes.Duration(1 * time.Second)
	_ = cfgContainer.Reload(cfg)

	resp = api.History(context.Background(), &HistoryRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestHistoryRemoveAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.HistoryRemove(context.Background(), &HistoryRemoveRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.HistoryRemove(context.Background(), &HistoryRemoveRequest{Channel: "test"})
	require.Equal(t, ErrorNotAvailable, resp.Error)

	cfg = cfgContainer.Config()
	cfg.Channel.WithoutNamespace.HistorySize = 1
	cfg.Channel.WithoutNamespace.HistoryTTL = configtypes.Duration(time.Second)
	_ = cfgContainer.Reload(cfg)

	resp = api.HistoryRemove(context.Background(), &HistoryRemoveRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestPresenceAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Presence(context.Background(), &PresenceRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.Presence(context.Background(), &PresenceRequest{Channel: "test"})

	require.Equal(t, ErrorNotAvailable, resp.Error)

	cfg = cfgContainer.Config()
	cfg.Channel.WithoutNamespace.Presence = true
	_ = cfgContainer.Reload(cfg)

	resp = api.Presence(context.Background(), &PresenceRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestPresenceStatsAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.PresenceStats(context.Background(), &PresenceStatsRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.PresenceStats(context.Background(), &PresenceStatsRequest{Channel: "test"})
	require.Equal(t, ErrorNotAvailable, resp.Error)

	cfg = cfgContainer.Config()
	cfg.Channel.WithoutNamespace.Presence = true
	_ = cfgContainer.Reload(cfg)

	resp = api.PresenceStats(context.Background(), &PresenceStatsRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestDisconnectAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Disconnect(context.Background(), &DisconnectRequest{
		User: "test",
	})
	require.Nil(t, resp.Error)
}

func TestUnsubscribeAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Unsubscribe(context.Background(), &UnsubscribeRequest{
		User:    "test",
		Channel: "test",
	})
	require.Nil(t, resp.Error)
}

func TestRefreshAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Refresh(context.Background(), &RefreshRequest{
		User: "test",
	})
	require.Nil(t, resp.Error)
}

func TestSubscribeAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Subscribe(context.Background(), &SubscribeRequest{
		User:    "test",
		Channel: "test",
	})
	require.Nil(t, resp.Error)
}

func TestInfoAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.Info(context.Background(), &InfoRequest{})
	require.Nil(t, resp.Error)
}

// configWithNamespace returns a DefaultConfig with a single namespace having the
// given name and subscription_type. Used to test the subscription_type gates on
// publish/map_*/shared_poll_* API endpoints. Fills the minimum mandatory fields
// (map.mode for map types, global shared_poll.hmac_secret_key for shared_poll)
// so the config passes Validate().
func configWithNamespace(name, subscriptionType string) config.Config {
	cfg := config.DefaultConfig()
	chOpts := configtypes.ChannelOptions{
		SubscriptionType: subscriptionType,
	}
	switch subscriptionType {
	case "map":
		// Persistent mode requires no key_ttl and is the simplest minimum.
		chOpts.Map = configtypes.MapConfig{Mode: "persistent"}
	case "map_clients", "map_users":
		// Persistent mode is forbidden here (presence entries need TTL-based
		// cleanup), so use ephemeral with a key_ttl.
		chOpts.Map = configtypes.MapConfig{
			Mode:   "ephemeral",
			KeyTTL: configtypes.Duration(time.Minute),
		}
	case "shared_poll":
		// Global HMAC secret required for any shared_poll namespace.
		cfg.SharedPoll.HMACSecretKey = "test-secret"
		// Default refresh proxy must have an endpoint when no explicit
		// proxy_name is set on the namespace.
		cfg.Channel.Proxy.SharedPollRefresh = configtypes.Proxy{
			Endpoint: "http://127.0.0.1:0/test",
			Timeout:  configtypes.Duration(time.Second),
		}
		chOpts.SharedPoll = configtypes.SharedPollConfig{Mode: "versioned"}
	}
	cfg.Channel.Namespaces = []configtypes.ChannelNamespace{
		{
			Name:           name,
			ChannelOptions: chOpts,
		},
	}
	return cfg
}

// TestPublishAPI_RejectsMapAndSharedPollNamespaces ensures the stream Publish
// endpoint rejects calls aimed at namespaces configured for map or shared poll
// subscriptions — those have their own dedicated APIs.
func TestPublishAPI_RejectsMapAndSharedPollNamespaces(t *testing.T) {
	cases := []struct {
		subscriptionType string
	}{
		{"map"},
		{"map_clients"},
		{"map_users"},
		{"shared_poll"},
	}
	for _, c := range cases {
		t.Run(c.subscriptionType, func(t *testing.T) {
			node := nodeWithMemoryEngine()
			defer func() { _ = node.Shutdown(context.Background()) }()

			cfg := configWithNamespace("ns", c.subscriptionType)
			cfgContainer, err := config.NewContainer(cfg)
			require.NoError(t, err)

			api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
			resp := api.Publish(context.Background(), &PublishRequest{Channel: "ns:test", Data: []byte("data")})
			require.Equal(t, ErrorBadRequest, resp.Error)
		})
	}
}

// TestMapPublishAPI_RejectsNonMapNamespaces ensures map_publish rejects calls
// against namespaces that aren't configured for map subscriptions.
func TestMapPublishAPI_RejectsNonMapNamespaces(t *testing.T) {
	cases := []struct {
		subscriptionType string
	}{
		{""},
		{"stream"},
		{"shared_poll"},
	}
	for _, c := range cases {
		t.Run("type="+c.subscriptionType, func(t *testing.T) {
			node := nodeWithMemoryEngine()
			defer func() { _ = node.Shutdown(context.Background()) }()

			cfg := configWithNamespace("ns", c.subscriptionType)
			cfgContainer, err := config.NewContainer(cfg)
			require.NoError(t, err)

			api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
			resp := api.MapPublish(context.Background(), &MapPublishRequest{
				Channel: "ns:test",
				Key:     "k",
				Data:    []byte(`{"v":1}`),
			})
			require.Equal(t, ErrorBadRequest, resp.Error)
		})
	}
}

// TestMapPublishAPI_RejectsInvalidKeyMode ensures key_mode is validated against
// the allowed set ("", "if_new", "if_exists").
func TestMapPublishAPI_RejectsInvalidKeyMode(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := configWithNamespace("ns", "map")
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})
	resp := api.MapPublish(context.Background(), &MapPublishRequest{
		Channel: "ns:test",
		Key:     "k",
		Data:    []byte(`{"v":1}`),
		KeyMode: "bogus",
	})
	require.Equal(t, ErrorBadRequest, resp.Error)
}

// TestMapAPIs_RejectNonMapNamespaces ensures the other map API endpoints
// (remove / read_state / read_stream / stats / clear) reject calls against
// namespaces that aren't configured for map subscriptions, instead of leaking
// internal errors from the broker.
func TestMapAPIs_RejectNonMapNamespaces(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := configWithNamespace("ns", "") // default = stream
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	rmResp := api.MapRemove(context.Background(), &MapRemoveRequest{Channel: "ns:test", Key: "k"})
	require.Equal(t, ErrorBadRequest, rmResp.Error)

	rsResp := api.MapReadState(context.Background(), &MapReadStateRequest{Channel: "ns:test"})
	require.Equal(t, ErrorBadRequest, rsResp.Error)

	rstrResp := api.MapReadStream(context.Background(), &MapReadStreamRequest{Channel: "ns:test"})
	require.Equal(t, ErrorBadRequest, rstrResp.Error)

	stResp := api.MapStats(context.Background(), &MapStatsRequest{Channel: "ns:test"})
	require.Equal(t, ErrorBadRequest, stResp.Error)

	clResp := api.MapClear(context.Background(), &MapClearRequest{Channel: "ns:test"})
	require.Equal(t, ErrorBadRequest, clResp.Error)
}

// TestMapAPIs_RejectUnknownChannel ensures the map API endpoints return
// ErrorUnknownChannel instead of leaking internal errors when the channel's
// namespace doesn't exist.
func TestMapAPIs_RejectUnknownChannel(t *testing.T) {
	node := nodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	api := NewExecutor(node, cfgContainer, &testSurveyCaller{}, ExecutorConfig{Protocol: "test", UseOpenTelemetry: false})

	// "missing:test" — namespace "missing" not configured.
	pubResp := api.MapPublish(context.Background(), &MapPublishRequest{Channel: "missing:test", Key: "k", Data: []byte(`{}`)})
	require.Equal(t, ErrorUnknownChannel, pubResp.Error)

	rmResp := api.MapRemove(context.Background(), &MapRemoveRequest{Channel: "missing:test", Key: "k"})
	require.Equal(t, ErrorUnknownChannel, rmResp.Error)

	rsResp := api.MapReadState(context.Background(), &MapReadStateRequest{Channel: "missing:test"})
	require.Equal(t, ErrorUnknownChannel, rsResp.Error)

	rstrResp := api.MapReadStream(context.Background(), &MapReadStreamRequest{Channel: "missing:test"})
	require.Equal(t, ErrorUnknownChannel, rstrResp.Error)

	stResp := api.MapStats(context.Background(), &MapStatsRequest{Channel: "missing:test"})
	require.Equal(t, ErrorUnknownChannel, stResp.Error)

	clResp := api.MapClear(context.Background(), &MapClearRequest{Channel: "missing:test"})
	require.Equal(t, ErrorUnknownChannel, clResp.Error)
}
