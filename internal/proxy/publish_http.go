package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// PublishRequestHTTP ...
type PublishRequestHTTP struct {
	baseRequestHTTP

	UserID  string `json:"user"`
	Channel string `json:"channel"`

	Data json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy binary data.
	Base64Data string `json:"b64data,omitempty"`
}

// HTTPPublishProxy ...
type HTTPPublishProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ PublishProxy = (*HTTPPublishProxy)(nil)

// NewHTTPPublishProxy ...
func NewHTTPPublishProxy(p Config) (*HTTPPublishProxy, error) {
	httpClient, err := proxyHTTPClient(p, "publish_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPPublishProxy{
		httpCaller: NewHTTPCaller(httpClient),
		config:     p,
	}, nil
}

// ProxyPublish proxies Publish to application backend.
func (p *HTTPPublishProxy) ProxyPublish(ctx context.Context, req *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
	data, err := httpEncoder.EncodePublishRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return transformPublishResponse(err, p.config.HTTP.StatusToCodeTransforms)
	}
	return httpDecoder.DecodePublishResponse(respData)
}

// Protocol ...
func (p *HTTPPublishProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPPublishProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPPublishProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
