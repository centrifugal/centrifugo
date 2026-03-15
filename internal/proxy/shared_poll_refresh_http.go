package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// HTTPSharedPollRefreshProxy ...
type HTTPSharedPollRefreshProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ SharedPollRefreshProxy = (*HTTPSharedPollRefreshProxy)(nil)

// NewHTTPSharedPollRefreshProxy ...
func NewHTTPSharedPollRefreshProxy(p Config) (*HTTPSharedPollRefreshProxy, error) {
	httpClient, err := proxyHTTPClient(p, "shared_poll_refresh_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPSharedPollRefreshProxy{
		httpCaller: NewHTTPCaller(httpClient),
		config:     p,
	}, nil
}

// ProxySharedPollRefresh proxies SharedPollRefresh to application backend.
func (p *HTTPSharedPollRefreshProxy) ProxySharedPollRefresh(ctx context.Context, req *proxyproto.SharedPollRefreshRequest) (*proxyproto.SharedPollRefreshResponse, error) {
	data, err := httpEncoder.EncodeSharedPollRefreshRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodeSharedPollRefreshResponse(respData)
}

// Protocol ...
func (p *HTTPSharedPollRefreshProxy) Protocol() string {
	return "http"
}
