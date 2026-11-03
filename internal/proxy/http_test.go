package proxy

import (
	"net/http"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func TestProxyHTTPClientMaxIdleConnsPerHost(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured int
		want       int
	}{
		{name: "unset", want: DefaultMaxIdleConnsPerHost},
		{name: "negative", configured: -1, want: DefaultMaxIdleConnsPerHost},
		{name: "configured", configured: 8192, want: 8192},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := configtypes.Proxy{Endpoint: "http://localhost:3000", Timeout: configtypes.Duration(time.Second)}
			p.HTTP.MaxIdleConnsPerHost = tc.configured
			client, err := proxyHTTPClient(p, "test")
			require.NoError(t, err)
			transport, ok := client.Transport.(*http.Transport)
			require.True(t, ok)
			require.Equal(t, tc.want, transport.MaxIdleConnsPerHost)
		})
	}
}
