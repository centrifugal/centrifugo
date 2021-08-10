package api

import (
	"context"
	"testing"
	"time"

	. "github.com/centrifugal/centrifugo/v3/internal/apiproto"
	"github.com/centrifugal/centrifugo/v3/internal/rule"
	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func nodeWithMemoryEngine() *centrifuge.Node {
	c := centrifuge.DefaultConfig
	n, err := centrifuge.New(c)
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

func (t testSurveyCaller) UserConnections(_ context.Context, _ *UserConnectionsRequest) (map[string]*UserConnectionInfo, error) {
	return nil, nil
}

func TestPublishAPI(t *testing.T) {
	node := nodeWithMemoryEngine()

	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
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
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.Broadcast(context.Background(), &BroadcastRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test"}})
	require.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test"}, Data: []byte("test")})
	require.Nil(t, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test:test"}, Data: []byte("test")})
	require.Nil(t, resp.Error)
	require.Equal(t, ErrorUnknownChannel, resp.Result.Responses[0].Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test", "test:test"}, Data: []byte("test")})
	require.Nil(t, resp.Error)
	require.Equal(t, ErrorUnknownChannel, resp.Result.Responses[1].Error)
}

func TestHistoryAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.History(context.Background(), &HistoryRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.History(context.Background(), &HistoryRequest{Channel: "test"})
	require.Equal(t, ErrorNotAvailable, resp.Error)

	config := ruleContainer.Config()
	config.HistorySize = 1
	config.HistoryTTL = tools.Duration(1 * time.Second)
	_ = ruleContainer.Reload(config)

	resp = api.History(context.Background(), &HistoryRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestHistoryRemoveAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.HistoryRemove(context.Background(), &HistoryRemoveRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.HistoryRemove(context.Background(), &HistoryRemoveRequest{Channel: "test"})
	require.Equal(t, ErrorNotAvailable, resp.Error)

	config := ruleContainer.Config()
	config.HistorySize = 1
	config.HistoryTTL = tools.Duration(time.Second)
	_ = ruleContainer.Reload(config)

	resp = api.HistoryRemove(context.Background(), &HistoryRemoveRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestPresenceAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.Presence(context.Background(), &PresenceRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.Presence(context.Background(), &PresenceRequest{Channel: "test"})

	require.Equal(t, ErrorNotAvailable, resp.Error)

	config := ruleContainer.Config()
	config.Presence = true
	_ = ruleContainer.Reload(config)

	resp = api.Presence(context.Background(), &PresenceRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestPresenceStatsAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.PresenceStats(context.Background(), &PresenceStatsRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.PresenceStats(context.Background(), &PresenceStatsRequest{Channel: "test"})
	require.Equal(t, ErrorNotAvailable, resp.Error)

	config := ruleContainer.Config()
	config.Presence = true
	_ = ruleContainer.Reload(config)

	resp = api.PresenceStats(context.Background(), &PresenceStatsRequest{Channel: "test"})
	require.Nil(t, resp.Error)
}

func TestDisconnectAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.Disconnect(context.Background(), &DisconnectRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.Disconnect(context.Background(), &DisconnectRequest{
		User: "test",
	})
	require.Nil(t, resp.Error)
}

func TestUnsubscribeAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.Unsubscribe(context.Background(), &UnsubscribeRequest{})
	require.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.Unsubscribe(context.Background(), &UnsubscribeRequest{
		User:    "test",
		Channel: "test",
	})
	require.Nil(t, resp.Error)
}

func TestInfoAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	ruleConfig := rule.DefaultConfig
	ruleContainer := rule.NewContainer(ruleConfig)

	api := NewExecutor(node, ruleContainer, &testSurveyCaller{}, "test")
	resp := api.Info(context.Background(), &InfoRequest{})
	require.Nil(t, resp.Error)
}
