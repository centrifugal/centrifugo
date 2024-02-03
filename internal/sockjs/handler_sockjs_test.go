package sockjs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifuge"

	"github.com/centrifugal/protocol"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func sockjsData(data []byte) []byte {
	quoted, _ := json.Marshal(string(data))
	return []byte(fmt.Sprintf("[%s]", string(quoted)))
}

func TestSockjsHandler(t *testing.T) {
	n, _ := centrifuge.New(centrifuge.Config{})
	require.NoError(t, n.Run())
	defer func() { _ = n.Shutdown(context.Background()) }()
	mux := http.NewServeMux()

	n.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		require.Equal(t, transportSockJS, event.Transport.Name())
		require.Equal(t, centrifuge.ProtocolTypeJSON, event.Transport.Protocol())
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{UserID: "user"},
			Data:        []byte(`{"SockJS connect response": 1}`),
		}, nil
	})

	doneCh := make(chan struct{})

	n.OnConnect(func(client *centrifuge.Client) {
		err := client.Send([]byte(`{"SockJS write": 1}`))
		require.NoError(t, err)
		client.Disconnect(centrifuge.DisconnectForceReconnect)
	})

	mux.Handle("/connection/sockjs/", NewHandler(n, Config{
		HandlerPrefix: "/connection/sockjs",
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	url := "ws" + server.URL[4:]

	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/sockjs/220/fi0988475/websocket", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.NotNil(t, conn)
	defer func() { _ = conn.Close() }()
	_, p, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "o", string(p)) // open frame of SockJS protocol.

	connectRequest := &protocol.ConnectRequest{
		Token: "boom",
	}
	cmd := &protocol.Command{
		Id:      1,
		Connect: connectRequest,
	}
	cmdBytes, _ := json.Marshal(cmd)
	err = conn.WriteMessage(websocket.TextMessage, sockjsData(cmdBytes))
	require.NoError(t, err)

	go func() {
		pos := 0
		contentExpected := []string{
			"SockJS connect response",
			"SockJS write",
			"force reconnect",
		}

	loop:
		for {
			_, p, err = conn.ReadMessage()
			if err != nil {
				break loop
			}

			for {
				if strings.Contains(string(p), contentExpected[pos]) {
					pos++
					if pos >= len(contentExpected) {
						close(doneCh)
						break loop
					}
				} else {
					break
				}
			}
		}
	}()

	waitWithTimeout(t, doneCh)
}

func waitWithTimeout(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		require.Fail(t, "timeout")
	}
}

func defaultNodeNoHandlers() *centrifuge.Node {
	n, err := centrifuge.New(centrifuge.Config{
		LogLevel:   centrifuge.LogLevelTrace,
		LogHandler: func(entry centrifuge.LogEntry) {},
	})
	if err != nil {
		panic(err)
	}
	err = n.Run()
	if err != nil {
		panic(err)
	}
	return n
}

func TestSockjsTransportWrite(t *testing.T) {
	node := defaultNodeNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		require.Equal(t, event.Transport.Protocol(), centrifuge.ProtocolTypeJSON)
		transport := event.Transport.(centrifuge.Transport)
		// Write to transport directly - this is only valid for tests, in normal situation
		// we write over client methods.
		require.NoError(t, transport.Write([]byte("hello")))
		return centrifuge.ConnectReply{}, centrifuge.DisconnectForceNoReconnect
	})

	mux := http.NewServeMux()
	mux.Handle("/connection/sockjs/", NewHandler(node, Config{
		HandlerPrefix: "/connection/sockjs",
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	url := "ws" + server.URL[4:]

	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/sockjs/220/fi0988475/websocket", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.NotNil(t, conn)
	defer func() { _ = conn.Close() }()
	_, p, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "o", string(p)) // open frame of SockJS protocol.

	connectRequest := &protocol.ConnectRequest{
		Token: "boom",
	}
	cmd := &protocol.Command{
		Id:      1,
		Connect: connectRequest,
	}
	cmdBytes, _ := json.Marshal(cmd)
	err = conn.WriteMessage(websocket.TextMessage, sockjsData(cmdBytes))
	require.NoError(t, err)

	_, p, err = conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "a[\"hello\"]", string(p))
}

func TestSockjsTransportWriteMany(t *testing.T) {
	node := defaultNodeNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		require.Equal(t, event.Transport.Protocol(), centrifuge.ProtocolTypeJSON)
		transport := event.Transport.(centrifuge.Transport)
		// Write to transport directly - this is only valid for tests, in normal situation
		// we write over client methods.
		require.NoError(t, transport.WriteMany([]byte("1"), []byte("22")))
		return centrifuge.ConnectReply{}, centrifuge.DisconnectForceNoReconnect
	})

	mux := http.NewServeMux()
	mux.Handle("/connection/sockjs/", NewHandler(node, Config{
		HandlerPrefix: "/connection/sockjs",
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	url := "ws" + server.URL[4:]

	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/sockjs/220/fi0988475/websocket", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.NotNil(t, conn)
	defer func() { _ = conn.Close() }()
	_, p, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "o", string(p)) // open frame of SockJS protocol.

	connectRequest := &protocol.ConnectRequest{
		Token: "boom",
	}
	cmd := &protocol.Command{
		Id:      1,
		Connect: connectRequest,
	}
	cmdBytes, _ := json.Marshal(cmd)
	err = conn.WriteMessage(websocket.TextMessage, sockjsData(cmdBytes))
	require.NoError(t, err)

	_, p, err = conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "a[\"1\\n22\"]", string(p))
}

func TestSockjsHandlerURLParams(t *testing.T) {
	n, _ := centrifuge.New(centrifuge.Config{})
	require.NoError(t, n.Run())
	defer func() { _ = n.Shutdown(context.Background()) }()
	mux := http.NewServeMux()

	n.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{UserID: "user"},
		}, nil
	})

	doneCh := make(chan struct{})

	n.OnConnect(func(client *centrifuge.Client) {
		require.Equal(t, transportSockJS, client.Transport().Name())
		require.Equal(t, centrifuge.ProtocolTypeJSON, client.Transport().Protocol())
		require.Equal(t, centrifuge.ProtocolVersion2, client.Transport().ProtocolVersion())
		close(doneCh)
	})

	mux.Handle("/connection/sockjs/", NewHandler(n, Config{
		HandlerPrefix: "/connection/sockjs",
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	url := "ws" + server.URL[4:]

	conn, resp, err := websocket.DefaultDialer.Dial(url+"/connection/sockjs/220/fi0988475/websocket?cf_protocol_version=v2", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.NotNil(t, conn)
	defer func() { _ = conn.Close() }()
	_, p, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "o", string(p)) // open frame of SockJS protocol.

	connectRequest := &protocol.ConnectRequest{
		Token: "boom",
	}
	cmd := &protocol.Command{
		Id:      1,
		Connect: connectRequest,
	}
	cmdBytes, _ := json.Marshal(cmd)
	err = conn.WriteMessage(websocket.TextMessage, sockjsData(cmdBytes))
	require.NoError(t, err)

	waitWithTimeout(t, doneCh)
}
