package unisse

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// All subscribers of a channel receive the same encoded publication: it is
// prepared once per protocol, not per connection. The handler must write it
// as is, not build a per-subscriber copy of it with the SSE framing around.
func TestUnidirectionalSSEDoesNotCopyMessages(t *testing.T) {
	node, err := centrifuge.New(centrifuge.Config{})
	require.NoError(t, err)
	node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		return centrifuge.ConnectReply{
			Credentials:   &centrifuge.Credentials{UserID: "user"},
			Subscriptions: map[string]centrifuge.SubscribeOptions{"news": {}},
		}, nil
	})
	require.NoError(t, node.Run())
	t.Cleanup(func() { _ = node.Shutdown(context.Background()) })

	handler := NewHandler(node, configtypes.UniSSE{MaxRequestBodySize: 65536}, centrifuge.PingPongConfig{
		PingInterval: time.Minute,
		PongTimeout:  10 * time.Second,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	const (
		numSubscribers = 10
		numMessages    = 50
		payloadSize    = 10 << 10
	)
	payload := []byte(`{"p":"` + strings.Repeat("x", payloadSize) + `"}`)

	received := make(chan struct{}, numSubscribers*(numMessages+1))
	for i := 0; i < numSubscribers; i++ {
		resp, err := http.Get(server.URL + "?" + connectUrlParam + "=" + url.QueryEscape("{}"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })

		connected := make(chan struct{})
		go func() {
			// ReadSlice and bytes.Contains don't allocate, so the readers
			// add next to nothing to the measurement below.
			reader := bufio.NewReaderSize(resp.Body, 2*payloadSize)
			first := true
			for {
				line, err := reader.ReadSlice('\n')
				if err != nil {
					return
				}
				if !bytes.HasPrefix(line, []byte("data: ")) {
					continue
				}
				if first {
					first = false
					close(connected)
					continue
				}
				if bytes.Contains(line, []byte(`"pub"`)) {
					received <- struct{}{}
				}
			}
		}()
		select {
		case <-connected:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for connect reply")
		}
	}

	publishAndWait := func(n int) {
		for i := 0; i < n; i++ {
			_, err := node.Publish("news", payload)
			require.NoError(t, err)
		}
		for i := 0; i < n*numSubscribers; i++ {
			select {
			case <-received:
			case <-time.After(5 * time.Second):
				t.Fatal("timeout waiting for publications")
			}
		}
	}
	publishAndWait(1) // Warm up.

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	publishAndWait(numMessages)
	runtime.ReadMemStats(&after)

	perDelivery := (after.TotalAlloc - before.TotalAlloc) / (numMessages * numSubscribers)
	t.Logf("heap bytes per delivered message: %d", perDelivery)
	// Encoding a publication once costs ~payloadSize/numSubscribers per
	// delivery; a copy per subscriber would add a whole payloadSize.
	require.Less(t, perDelivery, uint64(payloadSize/2))
}
