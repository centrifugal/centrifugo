package unisse

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/middleware"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/stretchr/testify/require"
)

func ensureSSEMessageHasClient(t *testing.T, data string) {
	t.Log(data)
	// SSE messages come in the format "data: {...}\n\n".
	if !strings.HasPrefix(data, "data: ") {
		t.Fatalf("Expected SSE message to start with 'data: ', got: %s (len: %d)", data, len(data))
	}
	jsonData := strings.TrimPrefix(data, "data: ")
	jsonData = strings.TrimSpace(jsonData)

	var reply protocol.Reply
	err := json.Unmarshal([]byte(jsonData), &reply)
	require.NoError(t, err)
	require.NotEmpty(t, reply.Connect.Client)
}

func readSSEMessage(reader *bufio.Reader) (string, error) {
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line marks end of SSE message
			if len(lines) > 0 {
				break
			}
			// Skip initial empty lines (like the \r\n at the beginning)
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func TestUnidirectionalSSE(t *testing.T) {
	t.Parallel()
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{},
		}, nil
	})

	config := configtypes.UniSSE{
		MaxRequestBodySize: 65536,
	}

	pingPong := centrifuge.PingPongConfig{
		PingInterval: 5 * time.Second,
		PongTimeout:  1 * time.Second,
	}

	handler := NewHandler(node, config, pingPong)

	server := httptest.NewServer(middleware.LogRequest(handler))
	t.Cleanup(func() { server.Close() })

	t.Run("successful connection with URL params", func(t *testing.T) {
		connectReq := `{}`
		params := url.Values{}
		params.Set(connectUrlParam, connectReq)

		resp, err := http.Get(server.URL + "?" + params.Encode())
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/event-stream; charset=utf-8", resp.Header.Get("Content-Type"))
		require.Contains(t, resp.Header.Get("Cache-Control"), "no-cache")

		reader := bufio.NewReader(resp.Body)
		message, err := readSSEMessage(reader)
		require.NoError(t, err)
		ensureSSEMessageHasClient(t, message)
	})

	t.Run("successful connection with POST body", func(t *testing.T) {
		connectReq := `{}`
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(connectReq))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/event-stream; charset=utf-8", resp.Header.Get("Content-Type"))

		reader := bufio.NewReader(resp.Body)
		message, err := readSSEMessage(reader)
		require.NoError(t, err)
		ensureSSEMessageHasClient(t, message)
	})

	t.Run("invalid connect request in URL params", func(t *testing.T) {
		params := url.Values{}
		params.Set(connectUrlParam, "invalid-json")

		resp, err := http.Get(server.URL + "?" + params.Encode())
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("invalid connect request in POST body", func(t *testing.T) {
		resp, err := http.Post(server.URL, "application/json", strings.NewReader("invalid-json"))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("unsupported method", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPut, server.URL, nil)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	t.Run("connect request without parameters", func(t *testing.T) {
		resp, err := http.Get(server.URL)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/event-stream; charset=utf-8", resp.Header.Get("Content-Type"))

		reader := bufio.NewReader(resp.Body)
		message, err := readSSEMessage(reader)
		require.NoError(t, err)
		ensureSSEMessageHasClient(t, message)
	})

	t.Run("ping cycle", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Get(server.URL)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		reader := bufio.NewReader(resp.Body)

		// Read connect response
		message, err := readSSEMessage(reader)
		require.NoError(t, err)
		ensureSSEMessageHasClient(t, message)

		for {
			message, err := readSSEMessage(reader)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(message, "data: {}") {
				t.Logf("centrifugal protocol ping received: %s", message)
				return
			}
		}
	})
}
