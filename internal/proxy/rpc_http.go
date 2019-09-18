package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/internal/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPRPCProxy ...
type HTTPRPCProxy struct {
	httpCaller HTTPCaller
	summary    prometheus.Observer
	errCount   prometheus.Counter
}

// RPCRequestHTTP ...
type RPCRequestHTTP struct {
	Protocol string          `json:"protocol"`
	Encoding string          `json:"encoding"`
	ClientID string          `json:"client"`
	UserID   string          `json:"user"`
	Data     json.RawMessage `json:"data,omitempty"`
	// Base64Data to proxy binary data.
	Base64Data string `json:"b64data,omitempty"`
}

// NewHTTPRPCProxy ...
func NewHTTPRPCProxy(endpoint string, httpClient *http.Client) *HTTPRPCProxy {
	return &HTTPRPCProxy{
		httpCaller: NewHTTPCaller(endpoint, httpClient),
		summary:    proxyCallDurationSummary.WithLabelValues("http", "rpc"),
		errCount:   proxyCallErrorCount.WithLabelValues("http", "rpc"),
	}
}

// ProxyRPC ...
func (p *HTTPRPCProxy) ProxyRPC(ctx context.Context, req RPCRequest) (*RPCReply, error) {
	httpRequest := middleware.HeadersFromContext(ctx)

	rpcHTTPReq := RPCRequestHTTP{
		Protocol: string(req.Transport.Protocol()),
		Encoding: string(req.Transport.Encoding()),
		ClientID: req.ClientID,
		UserID:   req.UserID,
	}

	if req.Transport.Encoding() == centrifuge.EncodingTypeJSON {
		rpcHTTPReq.Data = json.RawMessage(req.Data)
	} else {
		rpcHTTPReq.Base64Data = base64.StdEncoding.EncodeToString(req.Data)
	}

	data, err := json.Marshal(rpcHTTPReq)
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
	var res RPCReply
	err = json.Unmarshal(respData, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
