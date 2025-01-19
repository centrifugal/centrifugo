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

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
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

func getProxyHeader(allHeader http.Header, allowedHeaders []string, staticHeaders map[string]string, emulatedHeaders map[string]string) http.Header {
	proxyHeader := http.Header{}
	for k, v := range staticHeaders {
		proxyHeader.Set(k, v)
	}
	for k, v := range emulatedHeaders {
		proxyHeader.Set(k, v)
	}
	copyHeader(proxyHeader, allHeader, allowedHeaders)
	proxyHeader.Set("Content-Type", "application/json")
	return proxyHeader
}

func copyHeader(dst, src http.Header, extraHeaders []string) {
	for k, vv := range src {
		if !slices.Contains(extraHeaders, strings.ToLower(k)) {
			continue
		}
		dst[k] = vv
	}
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
