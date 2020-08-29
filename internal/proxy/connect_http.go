package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/centrifugal/centrifugo/internal/middleware"

	"github.com/centrifugal/centrifuge"
)

// ConnectRequestHTTP ...
type ConnectRequestHTTP struct {
	baseRequestHTTP

	Data json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy protobuf data.
	Base64Data string `json:"b64data,omitempty"`
	// Name of client.
	Name string `json:"name,omitempty"`
	// Version of client.
	Version string `json:"version,omitempty"`
}

// HTTPConnectProxy ...
type HTTPConnectProxy struct {
	httpCaller HTTPCaller
	options    Options
}

// NewHTTPConnectProxy ...
func NewHTTPConnectProxy(endpoint string, httpClient *http.Client, opts ...Option) *HTTPConnectProxy {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}
	return &HTTPConnectProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		options:    *options,
	}
}

// Protocol ...
func (p *HTTPConnectProxy) Protocol() string {
	return "http"
}

// ProxyConnect proxies connect control to application backend.
func (p *HTTPConnectProxy) ProxyConnect(ctx context.Context, req ConnectRequest) (*ConnectReply, error) {
	httpRequest := middleware.HeadersFromContext(ctx)

	connectHTTPReq := ConnectRequestHTTP{
		baseRequestHTTP: baseRequestHTTP{
			ClientID:  req.ClientID,
			Transport: req.Transport.Name(),
			Protocol:  string(req.Transport.Protocol()),
			Encoding:  string(req.Transport.Encoding()),
		},
		Name:    req.Name,
		Version: req.Version,
	}

	if req.Transport.Encoding() == centrifuge.EncodingTypeJSON {
		connectHTTPReq.Data = req.Data
	} else {
		connectHTTPReq.Base64Data = base64.StdEncoding.EncodeToString(req.Data)
	}

	data, err := json.Marshal(connectHTTPReq)
	if err != nil {
		return nil, err
	}

	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest, p.options.ExtraHeaders), data)
	if err != nil {
		return nil, err
	}

	var res ConnectReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
