package proxy

import (
	"context"
	"encoding/json"
	"time"

	"github.com/centrifugal/centrifugo/v4/internal/proxyproto"
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
	proxy      Proxy
	httpCaller HTTPCaller
}

var _ PublishProxy = (*HTTPPublishProxy)(nil)

// NewHTTPPublishProxy ...
func NewHTTPPublishProxy(p Proxy) (*HTTPPublishProxy, error) {
	return &HTTPPublishProxy{
		httpCaller: NewHTTPCaller(proxyHTTPClient(time.Duration(p.Timeout))),
		proxy:      p,
	}, nil
}

// ProxyPublish proxies Publish to application backend.
func (p *HTTPPublishProxy) ProxyPublish(ctx context.Context, req *proxyproto.PublishRequest) (*proxyproto.PublishResponse, error) {
	data, err := httpEncoder.EncodePublishRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.proxy.Endpoint, httpRequestHeaders(ctx, p.proxy), data)
	if err != nil {
		return nil, err
	}
	return httpDecoder.DecodePublishResponse(respData)
}

// Protocol ...
func (p *HTTPPublishProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPPublishProxy) UseBase64() bool {
	return p.proxy.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPPublishProxy) IncludeMeta() bool {
	return p.proxy.IncludeConnectionMeta
}
