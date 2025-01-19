package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/stretchr/testify/require"
)

func TestConnLimit_ConnectionRate(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	cfg := config.DefaultConfig()
	cfg.Client.ConnectionRateLimit = 10
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	ts := httptest.NewServer(NewConnLimit(node, cfgContainer).Middleware(testHandler()))
	defer ts.Close()

	for i := 0; i < 20; i++ {
		res, err := http.Post(ts.URL, "application/json", nil)
		require.NoError(t, err)
		if res.StatusCode == http.StatusServiceUnavailable {
			require.True(t, i >= 10)
			return
		}
	}
	require.Fail(t, "no rate limit hit upon sending 10 requests to a server")
}
