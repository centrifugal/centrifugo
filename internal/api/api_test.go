package api

import (
	"context"
	"testing"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/assert"
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

func TestPublishAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.Publish(context.Background(), &PublishRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{Channel: "test"})
	assert.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{Channel: "test", Data: []byte("test")})
	assert.Nil(t, resp.Error)

	resp = api.Publish(context.Background(), &PublishRequest{Channel: "test:test", Data: []byte("test")})
	assert.Equal(t, ErrorNamespaceNotFound, resp.Error)
}

func TestBroadcastAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.Broadcast(context.Background(), &BroadcastRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test"}})
	assert.Equal(t, ErrorBadRequest, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test"}, Data: []byte("test")})
	assert.Nil(t, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test:test"}, Data: []byte("test")})
	assert.Equal(t, ErrorNamespaceNotFound, resp.Error)

	resp = api.Broadcast(context.Background(), &BroadcastRequest{Channels: []string{"test", "test:test"}, Data: []byte("test")})
	assert.Equal(t, ErrorNamespaceNotFound, resp.Error)
}

func TestHistoryAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.History(context.Background(), &HistoryRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.History(context.Background(), &HistoryRequest{Channel: "test"})
	assert.Equal(t, ErrorNotAvailable, resp.Error)

	config := node.Config()
	config.HistorySize = 1
	config.HistoryLifetime = 1
	node.Reload(config)

	resp = api.History(context.Background(), &HistoryRequest{Channel: "test"})
	assert.Nil(t, resp.Error)
}

func TestHistoryRemoveAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.HistoryRemove(context.Background(), &HistoryRemoveRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.HistoryRemove(context.Background(), &HistoryRemoveRequest{Channel: "test"})
	assert.Equal(t, ErrorNotAvailable, resp.Error)

	config := node.Config()
	config.HistorySize = 1
	config.HistoryLifetime = 1
	node.Reload(config)

	resp = api.HistoryRemove(context.Background(), &HistoryRemoveRequest{Channel: "test"})
	assert.Nil(t, resp.Error)
}

func TestPresenceAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.Presence(context.Background(), &PresenceRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.Presence(context.Background(), &PresenceRequest{Channel: "test"})

	assert.Equal(t, ErrorNotAvailable, resp.Error)

	config := node.Config()
	config.Presence = true
	node.Reload(config)

	resp = api.Presence(context.Background(), &PresenceRequest{Channel: "test"})
	assert.Nil(t, resp.Error)
}

func TestPresenceStatsAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.PresenceStats(context.Background(), &PresenceStatsRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.PresenceStats(context.Background(), &PresenceStatsRequest{Channel: "test"})
	assert.Equal(t, ErrorNotAvailable, resp.Error)

	config := node.Config()
	config.Presence = true
	node.Reload(config)

	resp = api.PresenceStats(context.Background(), &PresenceStatsRequest{Channel: "test"})
	assert.Nil(t, resp.Error)
}

func TestDisconnectAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.Disconnect(context.Background(), &DisconnectRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.Disconnect(context.Background(), &DisconnectRequest{
		User: "test",
	})
	assert.Nil(t, resp.Error)
}

func TestUnsubscribeAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.Unsubscribe(context.Background(), &UnsubscribeRequest{})
	assert.Equal(t, ErrorBadRequest, resp.Error)
	resp = api.Unsubscribe(context.Background(), &UnsubscribeRequest{
		User:    "test",
		Channel: "test",
	})
	assert.Nil(t, resp.Error)
}

func TestChannelsAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.Channels(context.Background(), &ChannelsRequest{})
	assert.Nil(t, resp.Error)
}

func TestInfoAPI(t *testing.T) {
	node := nodeWithMemoryEngine()
	api := newAPIExecutor(node, "test")
	resp := api.Info(context.Background(), &InfoRequest{})
	assert.Nil(t, resp.Error)
}
