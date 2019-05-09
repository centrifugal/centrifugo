package proxy

import (
	"context"
	"encoding/json"
	"net/http"
)

// HTTPConnectProxy ...
type HTTPConnectProxy struct {
	Endpoint   string
	HTTPClient *http.Client
	httpCaller HTTPCaller
}

// NewHTTPConnectProxy ...
func NewHTTPConnectProxy(endpoint string, httpClient *http.Client) *HTTPConnectProxy {
	return &HTTPConnectProxy{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
		httpCaller: NewHTTPCaller(endpoint, httpClient),
	}
}

// ProxyConnect ...
func (p *HTTPConnectProxy) ProxyConnect(ctx context.Context, req ConnectRequest) (*ConnectResult, error) {
	httpRequest := req.Transport.Info().Request
	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest), nil)
	if err != nil {
		return nil, err
	}
	var res ConnectResult
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
