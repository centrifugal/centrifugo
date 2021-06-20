package proxy

import (
	"context"
	"encoding/json"

	"github.com/centrifugal/centrifugo/v3/internal/proxyproto"
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
	endpoint   string
	httpCaller HTTPCaller
	config     Config
}

var _ PublishProxy = (*HTTPPublishProxy)(nil)

// NewHTTPPublishProxy ...
func NewHTTPPublishProxy(endpoint string, config Config) (*HTTPPublishProxy, error) {
	return &HTTPPublishProxy{
		endpoint:   endpoint,
		httpCaller: NewHTTPCaller(proxyHTTPClient(config.PublishTimeout)),
		config:     config,
	}, nil
}

// ProxyPublish proxies Publish to application backend.
func (p *HTTPPublishProxy) ProxyPublish(ctx context.Context, req *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
	data, err := p.config.HTTPConfig.Encoder.EncodePublishRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return nil, err
	}
	return p.config.HTTPConfig.Decoder.DecodePublishResponse(respData)
}

// Protocol ...
func (p *HTTPPublishProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPPublishProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}
