package centrifuge

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestClientEventHub(t *testing.T) {
	h := ClientEventHub{}
	handler := func(e DisconnectEvent) DisconnectReply {
		return DisconnectReply{}
	}
	h.Disconnect(handler)
	assert.NotNil(t, h.disconnectHandler)
}

func TestSetCredentials(t *testing.T) {
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{})
	val := newCtx.Value(credentialsContextKey).(*Credentials)
	assert.NotNil(t, val)
}

func TestNewClient(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	client, err := newClient(context.Background(), node, transport)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestClientInitialState(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	assert.Equal(t, client.uid, client.ID())
	assert.NotNil(t, "", client.user)
	assert.Equal(t, 0, len(client.Channels()))
	assert.Equal(t, proto.EncodingJSON, client.Transport().Encoding())
	assert.Equal(t, "test_transport", client.Transport().Name())
	assert.False(t, client.closed)
	assert.False(t, client.authenticated)
	assert.Nil(t, client.disconnect)
}

func TestClientClosedState(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	err := client.close(nil)
	assert.NoError(t, err)
	assert.True(t, client.closed)
}

func TestClientConnectWithNoCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	_, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.NotNil(t, disconnect)
	assert.Equal(t, disconnect, DisconnectBadRequest)
}

func TestClientConnectWithWrongCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	_, disconnect := client.connectCmd(&proto.ConnectRequest{
		Credentials: &proto.SignedCredentials{
			User: "42",
			Exp:  "",
			Info: "",
			Sign: "",
		},
	})
	assert.NotNil(t, disconnect)
	assert.Equal(t, disconnect, DisconnectInvalidSign)
}

func TestClientConnectWithValidSignedCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.Secret = "secret"
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{
		Credentials: &proto.SignedCredentials{
			User: "42",
			Exp:  "1525541722",
			Info: "",
			Sign: "46789cda3bea52eca167961dd68c4d4240b9108f196a8516d1c753259b457d12",
		},
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, client.ID(), resp.Result.Client)
	assert.Equal(t, false, resp.Result.Expires)
}

func TestClientConnectWithExpiredSignedCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.Secret = "secret"
	node.config.ClientExpire = true
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{
		Credentials: &proto.SignedCredentials{
			User: "42",
			Exp:  "1525541722",
			Info: "",
			Sign: "46789cda3bea52eca167961dd68c4d4240b9108f196a8516d1c753259b457d12",
		},
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, true, resp.Result.Expires)
	assert.Equal(t, true, resp.Result.Expired)
	assert.Equal(t, uint32(0), resp.Result.TTL)
	assert.False(t, client.authenticated)
}

func TestClientRefreshSignedCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.ClientExpire = true
	node.config.Secret = "secret"
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{
		Credentials: &proto.SignedCredentials{
			User: "42",
			Exp:  "1525541722",
			Info: "",
			Sign: "46789cda3bea52eca167961dd68c4d4240b9108f196a8516d1c753259b457d12",
		},
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, "", resp.Result.Client)
	assert.Equal(t, true, resp.Result.Expires)
	assert.Equal(t, true, resp.Result.Expired)

	refreshResp, disconnect := client.refreshCmd(&proto.RefreshRequest{
		Credentials: &proto.SignedCredentials{
			User: "42",
			Exp:  "2525637058",
			Info: "",
			Sign: "1651ee08ba82f8e54578c4c20bc7675e6fce7af4b75f3e21aa537d4f0518a6a7",
		},
	})
	assert.Nil(t, disconnect)
	assert.NotEmpty(t, client.ID())
	assert.Equal(t, true, refreshResp.Result.Expires)
	assert.Equal(t, false, refreshResp.Result.Expired)
	assert.True(t, refreshResp.Result.TTL > 0)
}

func TestClientConnectContextCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.ClientExpire = true
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{
		UserID: "42",
		Exp:    time.Now().Unix() + 60,
	})
	client, _ := newClient(newCtx, node, transport)

	// Set refresh handler to tell library that server-side refresh must be used.
	client.On().Refresh(func(e RefreshEvent) RefreshReply {
		return RefreshReply{}
	})

	resp, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.Nil(t, disconnect)
	assert.Equal(t, false, resp.Result.Expires)
	assert.Equal(t, false, resp.Result.Expired)
	assert.Equal(t, uint32(0), resp.Result.TTL)
	assert.True(t, client.authenticated)
	assert.Equal(t, "42", client.UserID())
}

func TestClientConnectWithExpiredContextCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.Secret = "secret"
	node.config.ClientExpire = true
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{
		UserID: "42",
		Exp:    time.Now().Unix() - 60,
	})
	client, _ := newClient(newCtx, node, transport)

	// Set refresh handler to tell library that server-side refresh must be used.
	client.On().Refresh(func(e RefreshEvent) RefreshReply {
		return RefreshReply{}
	})

	_, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.NotNil(t, disconnect)
	assert.Equal(t, DisconnectExpired, disconnect)
}

func connectClient(t *testing.T, client *Client) *proto.ConnectResult {
	connectResp, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.Nil(t, disconnect)
	assert.Nil(t, connectResp.Error)
	assert.True(t, client.authenticated)
	assert.Equal(t, client.uid, connectResp.Result.Client)
	return connectResp.Result
}

func subscribeClient(t *testing.T, client *Client, ch string) *proto.SubscribeResult {
	subscribeResp, disconnect := client.subscribeCmd(&proto.SubscribeRequest{
		Channel: ch,
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, subscribeResp.Error)
	return subscribeResp.Result
}

func TestClientSubscribe(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	assert.Equal(t, 0, len(client.Channels()))

	subscribeResp, disconnect := client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "test1",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, subscribeResp.Error)
	assert.Empty(t, subscribeResp.Result.Last)
	assert.False(t, subscribeResp.Result.Recovered)
	assert.Empty(t, subscribeResp.Result.Publications)
	assert.Equal(t, 1, len(client.Channels()))

	subscribeResp, disconnect = client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "test2",
	})
	assert.Equal(t, 2, len(client.Channels()))

	assert.Equal(t, 1, node.Hub().NumClients())
	assert.Equal(t, 2, node.Hub().NumChannels())

	subscribeResp, disconnect = client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "test2",
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorAlreadySubscribed, subscribeResp.Error)
}

func TestClientSubscribeLast(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.HistorySize = 10
	node.config.HistoryLifetime = 60
	node.config.HistoryRecover = true

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	for i := 0; i < 10; i++ {
		node.Publish("test", &Publication{
			UID:  strconv.Itoa(i),
			Data: []byte("{}"),
		})
	}

	connectClient(t, client)
	result := subscribeClient(t, client, "test")
	assert.Equal(t, "9", result.Last)
}

func TestClientSubscribeRecover(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.HistorySize = 10
	node.config.HistoryLifetime = 60
	node.config.HistoryRecover = true

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	for i := 0; i < 10; i++ {
		node.Publish("test", &Publication{
			UID:  strconv.Itoa(i),
			Data: []byte(`{}`),
		})
	}

	connectClient(t, client)

	subscribeResp, disconnect := client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "test",
		Recover: true,
		Last:    "7",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, subscribeResp.Error)
	assert.Equal(t, 2, len(subscribeResp.Result.Publications))
	assert.True(t, subscribeResp.Result.Recovered)
}

func TestClientUnsubscribe(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)
	subscribeClient(t, client, "test")

	unsubscribeResp, disconnect := client.unsubscribeCmd(&proto.UnsubscribeRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, unsubscribeResp.Error)

	assert.Equal(t, 0, len(client.Channels()))
	assert.Equal(t, 1, node.Hub().NumClients())
	assert.Equal(t, 0, node.Hub().NumChannels())

	subscribeClient(t, client, "test")
	assert.Equal(t, 1, len(client.Channels()))

	err := client.Unsubscribe("test")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(client.Channels()))
	assert.Equal(t, 1, node.Hub().NumClients())
	assert.Equal(t, 0, node.Hub().NumChannels())
}

func TestClientPublish(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	publishResp, disconnect := client.publishCmd(&proto.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorPermissionDenied, publishResp.Error)

	node.config.Publish = true
	publishResp, disconnect = client.publishCmd(&proto.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, publishResp.Error)

	node.config.SubscribeToPublish = true
	publishResp, disconnect = client.publishCmd(&proto.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorPermissionDenied, publishResp.Error)

	subscribeClient(t, client, "test")
	publishResp, disconnect = client.publishCmd(&proto.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, publishResp.Error)
}

func TestClientPing(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	pingResp, disconnect := client.pingCmd(&proto.PingRequest{
		Data: "hi",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, pingResp.Error)
	assert.Equal(t, "hi", pingResp.Result.Data)
}

func TestClientPresence(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.Presence = true

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)
	subscribeClient(t, client, "test")

	presenceResp, disconnect := client.presenceCmd(&proto.PresenceRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, presenceResp.Error)
	assert.Equal(t, 1, len(presenceResp.Result.Presence))

	presenceStatsResp, disconnect := client.presenceStatsCmd(&proto.PresenceStatsRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, presenceResp.Error)
	assert.Equal(t, uint32(1), presenceStatsResp.Result.NumUsers)
	assert.Equal(t, uint32(1), presenceStatsResp.Result.NumClients)

	node.config.Presence = false
	presenceResp, disconnect = client.presenceCmd(&proto.PresenceRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorNotAvailable, presenceResp.Error)
	assert.Nil(t, presenceResp.Result)

	presenceStatsResp, disconnect = client.presenceStatsCmd(&proto.PresenceStatsRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorNotAvailable, presenceStatsResp.Error)
	assert.Nil(t, presenceStatsResp.Result)
}

func TestClientHistory(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.HistorySize = 10
	node.config.HistoryLifetime = 60

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	for i := 0; i < 10; i++ {
		node.Publish("test", &Publication{
			UID:  strconv.Itoa(i),
			Data: []byte(`{}`),
		})
	}

	connectClient(t, client)
	subscribeClient(t, client, "test")

	historyResp, disconnect := client.historyCmd(&proto.HistoryRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, historyResp.Error)
	assert.Equal(t, 10, len(historyResp.Result.Publications))

	node.config.HistorySize = 0
	node.config.HistoryLifetime = 0

	historyResp, disconnect = client.historyCmd(&proto.HistoryRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorNotAvailable, historyResp.Error)
	assert.Nil(t, historyResp.Result)
}

func TestClientCloseUnauthenticated(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.ClientStaleCloseDelay = time.Millisecond

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)
	time.Sleep(100 * time.Millisecond)
	assert.True(t, client.closed)
}

func TestClientPresenceUpdate(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.Presence = true

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)
	subscribeClient(t, client, "test")

	err := client.updateChannelPresence("test")
	assert.NoError(t, err)
}

func TestClientSend(t *testing.T) {
	node := nodeWithMemoryEngine()
	node.config.Presence = true

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	err := client.Send([]byte(`{}`))
	assert.NoError(t, err)

	err = transport.Close(nil)
	assert.NoError(t, err)

	err = client.Send([]byte(`{}`))
	assert.Error(t, err)
}

func TestClientClose(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	err := client.Close(DisconnectShutdown)
	assert.NoError(t, err)
	assert.True(t, transport.closed)
	assert.Equal(t, client.disconnect, DisconnectShutdown)
}

func TestClientHandleMalformedCommand(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	reply, disconnect := client.handle(&proto.Command{
		ID:     1,
		Method: 1000,
		Params: []byte(`{}`),
	})
	assert.Equal(t, ErrorMethodNotFound, reply.Error)
	assert.Nil(t, disconnect)

	reply, disconnect = client.handle(&proto.Command{
		ID:     1,
		Method: 2,
		Params: []byte(`{}`),
	})
	assert.Nil(t, reply)
	assert.Equal(t, DisconnectBadRequest, disconnect)
}
