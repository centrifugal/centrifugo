package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// MapPublishRequestHTTP ...
type MapPublishRequestHTTP struct {
	baseRequestHTTP

	UserID  string `json:"user"`
	Channel string `json:"channel"`
	Key     string `json:"key"`

	Data json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy binary data.
	Base64Data string `json:"b64data,omitempty"`
}

// HTTPMapPublishProxy ...
type HTTPMapPublishProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ MapPublishProxy = (*HTTPMapPublishProxy)(nil)

// NewHTTPMapPublishProxy ...
func NewHTTPMapPublishProxy(p Config) (*HTTPMapPublishProxy, error) {
	httpClient, err := proxyHTTPClient(p, "map_publish_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPMapPublishProxy{
		httpCaller: NewHTTPCaller(httpClient),
		config:     p,
	}, nil
}

// ProxyMapPublish proxies MapPublish to application backend.
func (p *HTTPMapPublishProxy) ProxyMapPublish(ctx context.Context, req *proxyproto.MapPublishRequest) (*proxyproto.MapPublishResponse, error) {
	data, err := httpEncoder.EncodeMapPublishRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return transformMapPublishResponse(err, p.config.HTTP.StatusToCodeTransforms)
	}
	return httpDecoder.DecodeMapPublishResponse(respData)
}

// Protocol ...
func (p *HTTPMapPublishProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPMapPublishProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPMapPublishProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
