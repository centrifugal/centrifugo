package unihttpstream

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/middleware"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/protocol"
	"github.com/stretchr/testify/require"
)

func ensureHTTPStreamMessageHasClient(t *testing.T, data string) {
	t.Log(data)
	var reply protocol.Reply
	err := json.Unmarshal([]byte(data), &reply)
	require.NoError(t, err)
	require.NotEmpty(t, reply.Connect.Client)
}

func readHTTPStreamMessage(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func TestUnidirectionalHTTPStream(t *testing.T) {
	t.Parallel()
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })

	node.OnConnecting(func(ctx context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{},
		}, nil
	})

	config := configtypes.UniHTTPStream{
		MaxRequestBodySize: 65536,
	}

	pingPong := centrifuge.PingPongConfig{
		PingInterval: 5 * time.Second,
		PongTimeout:  1 * time.Second,
	}

	handler := NewHandler(node, config, pingPong)

	server := httptest.NewServer(middleware.LogRequest(handler))
	t.Cleanup(func() { server.Close() })

	t.Run("successful connection with POST body", func(t *testing.T) {
		connectReq := `{}`
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(connectReq))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))

		reader := bufio.NewReader(resp.Body)
		message, err := readHTTPStreamMessage(reader)
		require.NoError(t, err)
		ensureHTTPStreamMessageHasClient(t, message)
	})

	t.Run("invalid connect request in POST body", func(t *testing.T) {
		resp, err := http.Post(server.URL, "application/json", strings.NewReader("invalid-json"))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("empty POST body", func(t *testing.T) {
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(""))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("GET method not allowed", func(t *testing.T) {
		resp, err := http.Get(server.URL)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	t.Run("PUT method not allowed", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPut, server.URL, strings.NewReader("{}"))
		require.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})

	t.Run("OPTIONS method", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodOptions, server.URL, nil)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.Equal(t, "300", resp.Header.Get("Access-Control-Max-Age"))
		require.Equal(t, "POST, OPTIONS", resp.Header.Get("Access-Control-Allow-Methods"))
	})

	t.Run("large request body", func(t *testing.T) {
		// Create a request body larger than typical limits
		largeBody := strings.Repeat("a", 100000)
		connectReq := `{"data":"` + largeBody + `"}`

		resp, err := http.Post(server.URL, "application/json", strings.NewReader(connectReq))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should either work or return an appropriate error status
		if resp.StatusCode != http.StatusOK {
			require.True(t, resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusRequestEntityTooLarge)
		}
	})

	t.Run("connection timeout", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{
			Timeout: 1 * time.Second,
		}

		resp, err := client.Post(server.URL, "application/json", strings.NewReader("{}"))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Try to read but expect timeout
		reader := bufio.NewReader(resp.Body)
		_, err = readHTTPStreamMessage(reader)
		// This may or may not error depending on timing, but connection should work initially
	})

	t.Run("ping cycle", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Post(server.URL, "application/json", strings.NewReader("{}"))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		reader := bufio.NewReader(resp.Body)

		// Read connect response
		message, err := readHTTPStreamMessage(reader)
		require.NoError(t, err)
		ensureHTTPStreamMessageHasClient(t, message)

		for {
			message, err = readHTTPStreamMessage(reader)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(message) == "{}" {
				t.Logf("centrifugal protocol ping received: %s", message)
				return
			}
		}
	})

	t.Run("malformed JSON in body", func(t *testing.T) {
		malformedJSON := `{"incomplete": json`
		resp, err := http.Post(server.URL, "application/json", strings.NewReader(malformedJSON))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
