package uniws

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/websocket"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// newCloseTestServer returns the websocket URL of a unidirectional websocket
// server, running onConnect for every connected client.
func newCloseTestServer(t *testing.T, onConnect func(*centrifuge.Client)) string {
	return newCloseTestServerPing(t, onConnect, centrifuge.PingPongConfig{})
}

// newCloseTestServerPing is newCloseTestServer with a ping configuration.
func newCloseTestServerPing(t *testing.T, onConnect func(*centrifuge.Client), pingPong centrifuge.PingPongConfig) string {
	t.Helper()
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })
	node.OnConnecting(func(context.Context, centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{Credentials: &centrifuge.Credentials{}}, nil
	})
	if onConnect != nil {
		node.OnConnect(onConnect)
	}
	require.NoError(t, node.Run())
	server := httptest.NewServer(NewHandler(node, Config{}, func(*http.Request) bool { return true }, pingPong))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

// dialConnected dials, sends the connect request and reads the connect reply.
// It also returns the TCP connection under the websocket one.
func dialConnected(t *testing.T, url string) (*websocket.Conn, net.Conn) {
	t.Helper()
	var raw net.Conn
	dialer := &websocket.Dialer{NetDial: func(network, addr string) (net.Conn, error) {
		c, err := net.Dial(network, addr)
		raw = c
		return c, err
	}}
	conn, _, _, err := dialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{}`)))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	ensureMessageHasClient(t, data)
	return conn, raw
}

// readCloseCode reads until the server's close frame, which the websocket
// library answers with the client's own, and returns its code.
func readCloseCode(t *testing.T, conn *websocket.Conn) int {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			require.True(t, errors.As(err, &closeErr), "expected a close frame, got %v", err)
			return closeErr.Code
		}
	}
}

// requireClosedPromptly asserts that the server closes the TCP connection
// promptly: the closing handshake completed, rather than the server waiting
// closeFrameWait for a close frame which never came.
func requireClosedPromptly(t *testing.T, raw net.Conn) {
	t.Helper()
	started := time.Now()
	require.NoError(t, raw.SetReadDeadline(time.Now().Add(2*closeFrameWait)))
	_, err := io.Copy(io.Discard, raw)
	require.NoError(t, err, "connection must end with EOF, not time out")
	require.Less(t, time.Since(started), closeFrameWait/2, "server waited for a close frame instead of completing the handshake")
}

func TestCloseHandshakeOnServerDisconnect(t *testing.T) {
	t.Parallel()
	url := newCloseTestServer(t, func(c *centrifuge.Client) {
		c.Disconnect(centrifuge.DisconnectConnectionLimit)
	})
	conn, raw := dialConnected(t, url)
	require.Equal(t, int(centrifuge.DisconnectConnectionLimit.Code), readCloseCode(t, conn))
	requireClosedPromptly(t, raw)
}

// fastPings makes pings frequent enough for a test to see several.
var fastPings = centrifuge.PingPongConfig{PingInterval: 300 * time.Millisecond, PongTimeout: 100 * time.Millisecond}

// A client answering every ping with an empty command, as the bidirectional
// protocol has clients do, keeps its connection: when the server later
// disconnects it, it gets that disconnect's code through a completed
// handshake.
func TestPingRepliesAllowed(t *testing.T) {
	t.Parallel()
	clients := make(chan *centrifuge.Client, 1)
	url := newCloseTestServerPing(t, func(c *centrifuge.Client) { clients <- c }, fastPings)
	conn, raw := dialConnected(t, url)
	client := <-clients

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	for pings := 0; pings < 3; {
		_, data, err := conn.ReadMessage()
		require.NoError(t, err)
		if string(data) == "{}" {
			pings++
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{}`)))
		}
	}
	client.Disconnect(centrifuge.DisconnectForceNoReconnect)
	require.Equal(t, int(centrifuge.DisconnectForceNoReconnect.Code), readCloseCode(t, conn))
	requireClosedPromptly(t, raw)
}

// A data message which is not a reply to a ping disconnects with
// DisconnectBadRequest, through a completed closing handshake.
func TestUnsolicitedDataMessageDisconnects(t *testing.T) {
	t.Parallel()
	url := newCloseTestServer(t, nil)
	conn, raw := dialConnected(t, url)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{}`)))
	require.Equal(t, int(centrifuge.DisconnectBadRequest.Code), readCloseCode(t, conn))
	requireClosedPromptly(t, raw)
}

// One reply per ping: a second one disconnects.
func TestSecondReplyToPingDisconnects(t *testing.T) {
	t.Parallel()
	url := newCloseTestServerPing(t, nil, fastPings)
	conn, raw := dialConnected(t, url)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	for {
		_, data, err := conn.ReadMessage()
		require.NoError(t, err)
		if string(data) == "{}" {
			break
		}
	}
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{}`)))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{}`)))
	require.Equal(t, int(centrifuge.DisconnectBadRequest.Code), readCloseCode(t, conn))
	requireClosedPromptly(t, raw)
}

// A client which keeps sending and never answers the close frame is not
// closing: the server stops reading it well before closeFrameWait.
func TestFloodIgnoringCloseCutOff(t *testing.T) {
	t.Parallel()
	url := newCloseTestServer(t, nil)
	conn, _ := dialConnected(t, url)

	started := time.Now()
	require.NoError(t, conn.SetWriteDeadline(started.Add(2*closeFrameWait)))
	for {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{}`)); err != nil {
			break
		}
	}
	require.Less(t, time.Since(started), closeFrameWait/2)
}
