package proxy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/centrifugal/centrifugo/internal/middleware"
)

// RefreshRequestHTTP ...
type RefreshRequestHTTP struct {
	baseRequestHTTP

	UserID string `json:"user"`
}

// HTTPRefreshProxy ...
type HTTPRefreshProxy struct {
	httpCaller HTTPCaller
	options    Options
}

// NewHTTPRefreshProxy ...
func NewHTTPRefreshProxy(endpoint string, httpClient *http.Client, opts ...Option) *HTTPRefreshProxy {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}
	return &HTTPRefreshProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		options:    *options,
	}
}

// ProxyRefresh proxies refresh to application backend.
func (p *HTTPRefreshProxy) ProxyRefresh(ctx context.Context, req RefreshRequest) (*RefreshReply, error) {
	httpRequest := middleware.HeadersFromContext(ctx)

	refreshHTTPReq := RefreshRequestHTTP{
		baseRequestHTTP: baseRequestHTTP{
			Transport: req.Transport.Name(),
			Protocol:  string(req.Transport.Protocol()),
			Encoding:  string(req.Transport.Encoding()),
			ClientID:  req.ClientID,
		},
		UserID: req.UserID,
	}

	data, err := json.Marshal(refreshHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest, p.options.ExtraHeaders), data)
	if err != nil {
		return nil, err
	}

	var res RefreshReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// Protocol ...
func (p *HTTPRefreshProxy) Protocol() string {
	return "http"
}
