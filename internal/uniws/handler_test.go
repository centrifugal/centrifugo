package uniws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/middleware"
	"github.com/centrifugal/centrifugo/v6/internal/websocket"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/stretchr/testify/require"
)

func ensureMessageHasClient(t *testing.T, data []byte) {
	t.Log(string(data))
	var reply protocol.Reply
	err := json.Unmarshal(data, &reply)
	require.NoError(t, err)
	require.NotEmpty(t, reply.Connect.Client)
}

func TestUnidirectionalWebSocket(t *testing.T) {
	t.Parallel()
	node, err := centrifuge.New(centrifuge.Config{
		ClientStaleCloseDelay: time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{},
		}, nil
	})

	config := configtypes.UniWebSocket{
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		WriteTimeout:     configtypes.Duration(time.Second),
		MessageSizeLimit: 65536,
	}

	pingPong := centrifuge.PingPongConfig{
		PingInterval: 5 * time.Second,
		PongTimeout:  1 * time.Second,
	}

	handler := NewHandler(node, config, func(r *http.Request) bool {
		t.Logf("Checking origin: %s", r.Header.Get("Origin"))
		return r.Header.Get("Origin") == "" || r.Header.Get("Origin") == "https://example.com"
	}, pingPong)

	server := httptest.NewServer(middleware.LogRequest(handler))
	t.Cleanup(func() { server.Close() })

	waitServer := func() {
		t.Helper()
		// Wait for server to start.
		for {
			t.Logf("Waiting for server to start: %s", server.URL)
			resp, err := http.Get(server.URL)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			_ = resp.Body.Close()
			break
		}
	}
	waitServer()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("successful upgrade", func(t *testing.T) {
		dialer := websocket.Dialer{}
		conn, _, _, err := dialer.Dial(wsURL, http.Header{"Origin": []string{"https://example.com"}})
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		connectMsg := `{}`
		err = conn.WriteMessage(websocket.TextMessage, []byte(connectMsg))
		require.NoError(t, err)

		_, data, err := conn.ReadMessage()
		require.NoError(t, err)
		ensureMessageHasClient(t, data)
	})

	t.Run("invalid connect request", func(t *testing.T) {
		dialer := websocket.Dialer{}
		conn, _, _, err := dialer.Dial(wsURL, http.Header{"Origin": []string{"https://example.com"}})
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		connectMsg := `invalid`
		err = conn.WriteMessage(websocket.TextMessage, []byte(connectMsg))
		require.NoError(t, err)

		_, _, err = conn.ReadMessage()
		require.Error(t, err)
	})

	t.Run("bad origin", func(t *testing.T) {
		dialer := websocket.Dialer{}
		_, _, _, err := dialer.Dial(wsURL, http.Header{"Origin": []string{"https://evil.com"}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "bad handshake", err.Error())
	})

	t.Run("connect request in URL params", func(t *testing.T) {
		connectReq := `{}`
		params := url.Values{}
		params.Set(connectUrlParam, connectReq)

		dialer := websocket.Dialer{}
		conn, _, _, err := dialer.Dial(wsURL+"?"+params.Encode(), nil)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		_, data, err := conn.ReadMessage()
		require.NoError(t, err)
		ensureMessageHasClient(t, data)
	})

	t.Run("invalid connect request in URL params", func(t *testing.T) {
		params := url.Values{}
		params.Set(connectUrlParam, "invalid-json")

		dialer := websocket.Dialer{}
		_, _, _, err := dialer.Dial(wsURL+"?"+params.Encode(), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "bad handshake", err.Error())
	})

	t.Run("connect request timeout", func(t *testing.T) {
		t.Parallel()
		dialer := websocket.Dialer{
			HandshakeTimeout: 1 * time.Second,
		}
		conn, _, _, err := dialer.Dial(wsURL, nil)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		started := time.Now()
		err = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		require.NoError(t, err)
		_, _, err = conn.ReadMessage()
		require.NoError(t, err) // Disconnect stale message.
		_, _, err = conn.ReadMessage()
		require.Error(t, err)
		require.WithinDuration(t, started, time.Now(), 3*time.Second)
	})

	t.Run("ping pong cycle", func(t *testing.T) {
		t.Parallel()
		dialer := websocket.Dialer{}
		conn, _, _, err := dialer.Dial(wsURL, nil)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		err = conn.WriteMessage(websocket.TextMessage, []byte(`{}`))
		require.NoError(t, err)

		pingReceived := false
		conn.SetPingHandler(func(appData []byte) error {
			pingReceived = true
			t.Log("websocket frame ping received")
			return conn.WriteMessage(websocket.PongMessage, appData)
		})

		err = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		require.NoError(t, err)

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if string(data) == `{}` {
				t.Logf("centrifugal protocol ping received: %s", string(data))
			}
		}

		require.True(t, pingReceived, "Expected to receive ping message")
	})
}

func TestUnidirectionalWebSocket_CloseFrameSent(t *testing.T) {
	t.Parallel()

	// Create a custom node to trigger server-side disconnect.
	testNode, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = testNode.Shutdown(context.Background()) })

	testNode.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{},
		}, nil
	})

	testNode.OnConnect(func(client *centrifuge.Client) {
		client.Disconnect(centrifuge.DisconnectConnectionLimit)
	})

	testHandler := NewHandler(testNode, Config{}, func(r *http.Request) bool {
		return true
	}, centrifuge.PingPongConfig{})

	testServer := httptest.NewServer(middleware.LogRequest(testHandler))
	t.Cleanup(func() { testServer.Close() })

	testWsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Wait for test server to start.
	for {
		resp, err := http.Get(testServer.URL)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		_ = resp.Body.Close()
		break
	}

	dialer := websocket.Dialer{}
	conn, _, _, err := dialer.Dial(testWsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Send connect request.
	err = conn.WriteMessage(websocket.TextMessage, []byte(`{}`))
	require.NoError(t, err)

	// Read connect reply.
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	ensureMessageHasClient(t, data)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))

	// Read until connection is closed - server should send close frame.
	var closeErr *websocket.CloseError
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			t.Logf("ReadMessage error: %v", err)
			closeErr = &websocket.CloseError{}
			if ok := errors.As(err, &closeErr); ok {
				t.Logf("Close frame received: code=%d, text=%s", closeErr.Code, closeErr.Text)
				break
			}
			// Some other error occurred.
			break
		}
	}

	// Verify close frame was received with correct code and reason.
	require.NotNil(t, closeErr, "Expected to receive close frame")
	require.Equal(t, centrifuge.DisconnectConnectionLimit.Code, uint32(closeErr.Code))
	require.Equal(t, centrifuge.DisconnectConnectionLimit.Reason, closeErr.Text)
}
