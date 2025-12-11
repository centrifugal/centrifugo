package api

import (
	"context"
	"testing"
	"time"

	. "github.com/centrifugal/centrifugo/v6/internal/apiproto"
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

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
