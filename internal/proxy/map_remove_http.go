package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// MapRemoveRequestHTTP ...
type MapRemoveRequestHTTP struct {
	baseRequestHTTP

	UserID  string `json:"user"`
	Channel string `json:"channel"`
	Key     string `json:"key"`
}

// HTTPMapRemoveProxy ...
type HTTPMapRemoveProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ MapRemoveProxy = (*HTTPMapRemoveProxy)(nil)

// NewHTTPMapRemoveProxy ...
func NewHTTPMapRemoveProxy(p Config) (*HTTPMapRemoveProxy, error) {
	httpClient, err := proxyHTTPClient(p, "map_remove_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPMapRemoveProxy{
		httpCaller: NewHTTPCaller(httpClient),
		config:     p,
	}, nil
}

// ProxyMapRemove proxies MapRemove to application backend.
func (p *HTTPMapRemoveProxy) ProxyMapRemove(ctx context.Context, req *proxyproto.MapRemoveRequest) (*proxyproto.MapRemoveResponse, error) {
	data, err := httpEncoder.EncodeMapRemoveRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return transformMapRemoveResponse(err, p.config.HTTP.StatusToCodeTransforms)
	}
	return httpDecoder.DecodeMapRemoveResponse(respData)
}

// Protocol ...
func (p *HTTPMapRemoveProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPMapRemoveProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPMapRemoveProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
