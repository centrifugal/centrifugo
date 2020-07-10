package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/centrifugal/centrifuge"

	"github.com/centrifugal/centrifugo/internal/middleware"
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
	httpCaller HTTPCaller
	options    Options
}

// NewHTTPPublishProxy ...
func NewHTTPPublishProxy(endpoint string, httpClient *http.Client, opts ...Option) *HTTPPublishProxy {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}
	return &HTTPPublishProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		options:    *options,
	}
}

// ProxyPublish proxies Publish to application backend.
func (p *HTTPPublishProxy) ProxyPublish(ctx context.Context, req PublishRequest) (*PublishReply, error) {
	httpRequest := middleware.HeadersFromContext(ctx)

	publishHTTPReq := PublishRequestHTTP{
		baseRequestHTTP: baseRequestHTTP{
			Transport: req.Transport.Name(),
			Protocol:  string(req.Transport.Protocol()),
			Encoding:  string(req.Transport.Encoding()),
			ClientID:  req.ClientID,
		},
		UserID:  req.UserID,
		Channel: req.Channel,
	}

	if req.Transport.Encoding() == centrifuge.EncodingTypeJSON {
		publishHTTPReq.Data = json.RawMessage(req.Data)
	} else {
		publishHTTPReq.Base64Data = base64.StdEncoding.EncodeToString(req.Data)
	}

	data, err := json.Marshal(publishHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest, p.options.ExtraHeaders), data)
	if err != nil {
		return nil, err
	}

	var res PublishReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Protocol ...
func (p *HTTPPublishProxy) Protocol() string {
	return "http"
}
