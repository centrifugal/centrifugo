package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/centrifugal/centrifugo/internal/middleware"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

// ConnectRequestHTTP ...
type ConnectRequestHTTP struct {
	ClientID  string          `json:"client"`
	Transport string          `json:"transport"`
	Protocol  string          `json:"protocol"`
	Encoding  string          `json:"encoding"`
	Data      json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy protobuf data.
	Base64Data string `json:"b64data,omitempty"`
}

// HTTPConnectProxy ...
type HTTPConnectProxy struct {
	httpCaller HTTPCaller
	summary    prometheus.Observer
	errCount   prometheus.Counter
}

// NewHTTPConnectProxy ...
func NewHTTPConnectProxy(endpoint string, httpClient *http.Client) *HTTPConnectProxy {
	return &HTTPConnectProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		summary:    proxyCallDurationSummary.WithLabelValues("http", "connect"),
		errCount:   proxyCallErrorCount.WithLabelValues("http", "connect"),
	}
}

// ProxyConnect proxies connect control to application backend.
func (p *HTTPConnectProxy) ProxyConnect(ctx context.Context, req ConnectRequest) (*ConnectReply, error) {
	httpRequest := middleware.HeadersFromContext(ctx)

	connectHTTPReq := ConnectRequestHTTP{
		ClientID:  req.ClientID,
		Transport: req.Transport.Name(),
		Protocol:  string(req.Transport.Protocol()),
		Encoding:  string(req.Transport.Encoding()),
	}

	if req.Transport.Encoding() == centrifuge.EncodingTypeJSON {
		connectHTTPReq.Data = json.RawMessage(req.Data)
	} else {
		connectHTTPReq.Base64Data = base64.StdEncoding.EncodeToString(req.Data)
	}

	data, err := json.Marshal(connectHTTPReq)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	respData, err := p.httpCaller.CallHTTP(ctx, getProxyHeader(httpRequest), data)
	p.summary.Observe(time.Since(started).Seconds())
	if err != nil {
		p.errCount.Inc()
		return nil, err
	}
	var res ConnectReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
