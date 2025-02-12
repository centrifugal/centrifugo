package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/clientcontext"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/middleware"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc/metadata"
)

type baseRequestHTTP struct {
	ClientID  string `json:"client"`
	Transport string `json:"transport"`
	Protocol  string `json:"protocol"`
	Encoding  string `json:"encoding"`
}

var httpEncoder = &proxyproto.JSONEncoder{}
var httpDecoder = &proxyproto.JSONDecoder{}

// DefaultMaxIdleConnsPerHost is a reasonable value for all HTTP clients.
const DefaultMaxIdleConnsPerHost = 255

// HTTPCaller is responsible for calling HTTP.
type HTTPCaller interface {
	CallHTTP(context.Context, string, http.Header, []byte) ([]byte, error)
}

type httpCaller struct {
	Endpoint   string
	HTTPClient *http.Client
}

// NewHTTPCaller creates new HTTPCaller.
func NewHTTPCaller(httpClient *http.Client) HTTPCaller {
	return &httpCaller{
		HTTPClient: httpClient,
	}
}

func proxyHTTPClient(p configtypes.Proxy, logTraceEntity string) (*http.Client, error) {
	var tlsConfig *tls.Config
	if p.HTTP.TLS.Enabled {
		var err error
		tlsConfig, err = p.HTTP.TLS.ToGoTLSConfig(logTraceEntity)
		if err != nil {
			return nil, fmt.Errorf("error creating TLS config: %w", err)
		}
	}
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
			TLSClientConfig:     tlsConfig,
		},
		Timeout: p.Timeout.ToDuration(),
	}, nil
}

type statusCodeError struct {
	Code int
}

func (e *statusCodeError) Error() string {
	return fmt.Sprintf("unexpected HTTP status code: %d", e.Code)
}

func (c *httpCaller) CallHTTP(ctx context.Context, endpoint string, header http.Header, reqData []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("error constructing HTTP request: %w", err)
	}
	req.Header = header
	resp, err := c.HTTPClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("HTTP request error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusCodeError{resp.StatusCode}
	}
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading HTTP body: %w", err)
	}
	return respData, nil
}

func transformHTTPStatusError(err error, transforms []configtypes.HttpStatusToCodeTransform) (*proxyproto.Error, *proxyproto.Disconnect) {
	if len(transforms) == 0 {
		return nil, nil
	}
	var statusErr *statusCodeError
	if !errors.As(err, &statusErr) {
		return nil, nil
	}
	for _, t := range transforms {
		if t.StatusCode == statusErr.Code {
			if t.ToError.Code > 0 {
				return &proxyproto.Error{
					Code:      t.ToError.Code,
					Message:   t.ToError.Message,
					Temporary: t.ToError.Temporary,
				}, nil
			}
			if t.ToDisconnect.Code > 0 {
				return nil, &proxyproto.Disconnect{
					Code:   t.ToDisconnect.Code,
					Reason: t.ToDisconnect.Reason,
				}
			}
		}
	}
	return nil, nil
}

func httpRequestHeaders(ctx context.Context, proxy Config) http.Header {
	return requestHeaders(ctx, proxy.HttpHeaders, proxy.GrpcMetadata, proxy.HTTP.StaticHeaders)
}

func requestHeaders(ctx context.Context, allowedHeaders, allowedMetaKeys []string, staticHeaders map[string]string) http.Header {
	headers := http.Header{}

	emulatedHeaders, _ := clientcontext.GetEmulatedHeadersFromContext(ctx)
	for k, v := range emulatedHeaders {
		if slices.Contains(allowedHeaders, strings.ToLower(k)) {
			headers.Set(k, v)
		}
	}

	httpHeaders, hasHTTPHeaders := middleware.GetHeadersFromContext(ctx)
	for k, vv := range httpHeaders {
		if slices.Contains(allowedHeaders, strings.ToLower(k)) {
			headers[k] = vv
		}
	}

	if !hasHTTPHeaders {
		md, _ := metadata.FromIncomingContext(ctx)
		for k, vv := range md {
			if slices.Contains(allowedMetaKeys, k) {
				headers[k] = vv
			}
		}
	}

	for k, v := range staticHeaders {
		headers.Set(k, v)
	}

	headers.Set("Content-Type", "application/json")

	return headers
}
