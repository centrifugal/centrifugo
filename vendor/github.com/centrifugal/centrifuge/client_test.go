package centrifuge

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge/internal/proto"
	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
)

func getConnToken(user string, exp int64) string {
	claims := jwt.MapClaims{"user": user}
	if exp > 0 {
		claims["exp"] = exp
	}
	t, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		panic(err)
	}
	return t
}

func getSubscribeToken(channel string, client string, exp int64) string {
	claims := jwt.MapClaims{"channel": channel, "client": client}
	if exp > 0 {
		claims["exp"] = exp
	}
	t, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		panic(err)
	}
	return t
}

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

func TestClientConnectNoCredentialsNoToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	_, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.NotNil(t, disconnect)
	assert.Equal(t, disconnect, DisconnectBadRequest)
}

func TestClientConnectNoCredentialsNoTokenInsecure(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.ClientInsecure = true
	node.Reload(config)

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.Nil(t, disconnect)
	assert.Nil(t, resp.Error)
	assert.NotEmpty(t, resp.Result.Client)
	assert.Empty(t, client.UserID())
}

func TestClientConnectNoCredentialsNoTokenAnonymous(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.ClientAnonymous = true
	node.Reload(config)

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.Nil(t, disconnect)
	assert.Nil(t, resp.Error)
	assert.NotEmpty(t, resp.Result.Client)
	assert.Empty(t, client.UserID())
}

func TestClientConnectWithMalformedToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	_, disconnect := client.connectCmd(&proto.ConnectRequest{
		Token: "bad bad token",
	})
	assert.NotNil(t, disconnect)
	assert.Equal(t, disconnect, DisconnectInvalidToken)
}

func TestClientConnectWithValidToken(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.Secret = "secret"
	node.Reload(config)

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{
		Token: getConnToken("42", 0),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, client.ID(), resp.Result.Client)
	assert.Equal(t, false, resp.Result.Expires)
}

func TestClientConnectWithExpiringToken(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.Secret = "secret"
	node.Reload(config)

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{
		Token: getConnToken("42", time.Now().Unix()+10),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, true, resp.Result.Expires)
	assert.True(t, resp.Result.TTL > 0)
	assert.True(t, client.authenticated)
}

func TestClientConnectWithExpiredToken(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.Secret = "secret"
	node.Reload(config)

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{
		Token: getConnToken("42", 1525541722),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorTokenExpired, resp.Error)
	assert.False(t, client.authenticated)
}

func TestClientTokenRefresh(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.Secret = "secret"
	node.Reload(config)

	transport := newTestTransport()
	client, _ := newClient(context.Background(), node, transport)
	resp, disconnect := client.connectCmd(&proto.ConnectRequest{
		Token: getConnToken("42", 1525541722),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorTokenExpired, resp.Error)

	refreshResp, disconnect := client.refreshCmd(&proto.RefreshRequest{
		Token: getConnToken("42", 2525637058),
	})
	assert.Nil(t, disconnect)
	assert.NotEmpty(t, client.ID())
	assert.True(t, refreshResp.Result.Expires)
	assert.True(t, refreshResp.Result.TTL > 0)
}

func TestClientConnectContextCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	node.Reload(config)

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{
		UserID:   "42",
		ExpireAt: time.Now().Unix() + 60,
	})
	client, _ := newClient(newCtx, node, transport)

	// Set refresh handler to tell library that server-side refresh must be used.
	client.On().Refresh(func(e RefreshEvent) RefreshReply {
		return RefreshReply{}
	})

	resp, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.Nil(t, disconnect)
	assert.Equal(t, false, resp.Result.Expires)
	assert.Equal(t, uint32(0), resp.Result.TTL)
	assert.True(t, client.authenticated)
	assert.Equal(t, "42", client.UserID())
}

func TestClientConnectWithExpiredContextCredentials(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	node.Reload(config)

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{
		UserID:   "42",
		ExpireAt: time.Now().Unix() - 60,
	})
	client, _ := newClient(newCtx, node, transport)

	// Set refresh handler to tell library that server-side refresh must be used.
	client.On().Refresh(func(e RefreshEvent) RefreshReply {
		return RefreshReply{}
	})

	resp, disconnect := client.connectCmd(&proto.ConnectRequest{})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorExpired, resp.Error)
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

func TestClientSubscribePrivateChannelNoToken(t *testing.T) {
	node := nodeWithMemoryEngine()
	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	subscribeResp, disconnect := client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "$test1",
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorPermissionDenied, subscribeResp.Error)
}

func TestClientSubscribePrivateChannelWithToken(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.Secret = "secret"
	node.Reload(config)

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	subscribeResp, disconnect := client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeToken("$wrong_channel", "wrong client", 0),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorPermissionDenied, subscribeResp.Error)

	subscribeResp, disconnect = client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeToken("$wrong_channel", client.ID(), 0),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorPermissionDenied, subscribeResp.Error)

	subscribeResp, disconnect = client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeToken("$test1", client.ID(), 0),
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, subscribeResp.Error)
}

func TestClientSubscribePrivateChannelWithExpiringToken(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.Secret = "secret"
	node.Reload(config)

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)

	connectClient(t, client)

	subscribeResp, disconnect := client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeToken("$test1", client.ID(), 10),
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorTokenExpired, subscribeResp.Error)

	subscribeResp, disconnect = client.subscribeCmd(&proto.SubscribeRequest{
		Channel: "$test1",
		Token:   getSubscribeToken("$test1", client.ID(), time.Now().Unix()+10),
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, subscribeResp.Error, "token is valid and not expired yet")
	assert.True(t, subscribeResp.Result.Expires, "expires flag must be set")
	assert.True(t, subscribeResp.Result.TTL > 0, "positive TTL must be set")
}

func TestClientSubscribeLast(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.HistorySize = 10
	config.HistoryLifetime = 60
	config.HistoryRecover = true
	node.Reload(config)

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

var recoverTests = []struct {
	Name            string
	HistorySize     int
	HistoryLifetime int
	NumPublications int
	Last            string
	Away            uint32
	NumRecovered    int
	Recovered       bool
}{
	{"from_last_uid", 10, 60, 10, "7", 0, 2, true},
	{"empty_last_uid_full_history", 10, 60, 10, "", 0, 10, false},
	{"empty_last_uid_short_disconnect", 10, 60, 9, "", 19, 9, true},
	{"empty_last_uid_long_disconnect", 10, 60, 9, "", 119, 9, false},
}

func TestClientSubscribeRecover(t *testing.T) {
	for _, tt := range recoverTests {
		t.Run(tt.Name, func(t *testing.T) {
			node := nodeWithMemoryEngine()

			config := node.Config()
			config.HistorySize = tt.HistorySize
			config.HistoryLifetime = tt.HistoryLifetime
			config.HistoryRecover = true
			node.Reload(config)

			transport := newTestTransport()
			ctx := context.Background()
			newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
			client, _ := newClient(newCtx, node, transport)

			for i := 0; i < tt.NumPublications; i++ {
				node.Publish("test", &Publication{
					UID:  strconv.Itoa(i),
					Data: []byte(`{}`),
				})
			}

			connectClient(t, client)

			subscribeResp, disconnect := client.subscribeCmd(&proto.SubscribeRequest{
				Channel: "test",
				Recover: true,
				Last:    tt.Last,
				Away:    tt.Away,
			})
			assert.Nil(t, disconnect)
			assert.Nil(t, subscribeResp.Error)
			assert.Equal(t, tt.NumRecovered, len(subscribeResp.Result.Publications))
			assert.Equal(t, tt.Recovered, subscribeResp.Result.Recovered)
		})
	}
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

	err := client.Unsubscribe("test", false)
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

	config := node.Config()
	config.Publish = true
	node.Reload(config)

	publishResp, disconnect = client.publishCmd(&proto.PublishRequest{
		Channel: "test",
		Data:    []byte(`{}`),
	})
	assert.Nil(t, disconnect)
	assert.Nil(t, publishResp.Error)

	config = node.Config()
	config.SubscribeToPublish = true
	node.Reload(config)

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

	config := node.Config()
	config.Presence = true
	node.Reload(config)

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

	config = node.Config()
	config.Presence = false
	node.Reload(config)

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

	config := node.Config()
	config.HistorySize = 10
	config.HistoryLifetime = 60
	node.Reload(config)

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

	config = node.Config()
	config.HistorySize = 0
	config.HistoryLifetime = 0
	node.Reload(config)

	historyResp, disconnect = client.historyCmd(&proto.HistoryRequest{
		Channel: "test",
	})
	assert.Nil(t, disconnect)
	assert.Equal(t, ErrorNotAvailable, historyResp.Error)
	assert.Nil(t, historyResp.Result)
}

func TestClientCloseUnauthenticated(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.ClientStaleCloseDelay = time.Millisecond
	node.Reload(config)

	transport := newTestTransport()
	ctx := context.Background()
	newCtx := SetCredentials(ctx, &Credentials{UserID: "42"})
	client, _ := newClient(newCtx, node, transport)
	time.Sleep(100 * time.Millisecond)
	client.mu.Lock()
	assert.True(t, client.closed)
	client.mu.Unlock()
}

func TestClientPresenceUpdate(t *testing.T) {
	node := nodeWithMemoryEngine()

	config := node.Config()
	config.Presence = true
	node.Reload(config)

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
