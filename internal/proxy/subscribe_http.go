package proxy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/centrifugal/centrifugo/internal/middleware"
)

// SubscribeRequestHTTP ...
type SubscribeRequestHTTP struct {
	baseRequestHTTP

	UserID  string `json:"user"`
	Channel string `json:"channel"`
}

// HTTPSubscribeProxy ...
type HTTPSubscribeProxy struct {
	httpCaller HTTPCaller
	options    Options
}

// NewHTTPSubscribeProxy ...
func NewHTTPSubscribeProxy(endpoint string, httpClient *http.Client, opts ...Option) *HTTPSubscribeProxy {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}
	return &HTTPSubscribeProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		options:    *options,
	}
}

// ProxySubscribe proxies Subscribe to application backend.
func (p *HTTPSubscribeProxy) ProxySubscribe(ctx context.Context, req SubscribeRequest) (*SubscribeReply, error) {
	httpRequest := middleware.HeadersFromContext(ctx)

	subscribeHTTPReq := SubscribeRequestHTTP{
		baseRequestHTTP: baseRequestHTTP{
			Transport: req.Transport.Name(),
			Protocol:  string(req.Transport.Protocol()),
			Encoding:  string(req.Transport.Encoding()),
			ClientID:  req.ClientID,
		},
		UserID:  req.UserID,
		Channel: req.Channel,
	}

	data, err := json.Marshal(subscribeHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest, p.options.ExtraHeaders), data)
	if err != nil {
		return nil, err
	}

	var res SubscribeReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Protocol ...
func (p *HTTPSubscribeProxy) Protocol() string {
	return "http"
}
