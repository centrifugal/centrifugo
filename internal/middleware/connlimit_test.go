package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifugo/v5/internal/rule"
	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/stretchr/testify/require"
)

func TestConnLimit_ConnectionRate(t *testing.T) {
	node := tools.NodeWithMemoryEngineNoHandlers()
	defer func() { _ = node.Shutdown(context.Background()) }()

	ruleConfig, err := rule.NewContainer(rule.Config{
		ClientConnectionRateLimit: 10,
	})
	require.NoError(t, err)

	ts := httptest.NewServer(NewConnLimit(node, ruleConfig).Middleware(testHandler()))
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
