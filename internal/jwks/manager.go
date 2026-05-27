package jwks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/rakutentech/jwk-go/jwk"
	"github.com/rs/zerolog/log"
	"github.com/valyala/fasttemplate"
	"golang.org/x/sync/singleflight"
)

const (
	_defaultRetries            = 2
	_defaultTimeout            = 1 * time.Second
	_defaultMaxIdleConnPerHost = 255
	_defaultTTL                = 1 * time.Hour
)

// JWK represents an unparsed JSON Web Key (JWK) in its wire format.
type JWK = jwk.JWK

var (
	// ErrInvalidURL returned when input url has invalid format.
	ErrInvalidURL = errors.New("jwks: invalid url value or format")
	// ErrInvalidNumRetries returned when number of retries is zero.
	ErrInvalidNumRetries = errors.New("jwks: invalid number of retries")
	// ErrKeyIDNotProvided returned when input kid is not present.
	ErrKeyIDNotProvided = errors.New("jwks: kid is not provided")
	// ErrPublicKeyNotFound returned when no public key is found.
	ErrPublicKeyNotFound = errors.New("jwks: public key not found")

	errUnexpectedStatusCode = errors.New("jwks: unexpected status code")
	errUnmarshal            = errors.New("jwks: unmarshal error")
	errConvert              = errors.New("jwks: convert error")
)

// Manager fetches and returns JWK from public source.
type Manager struct {
	url      *fasttemplate.Template
	cache    Cache
	client   *http.Client
	useCache bool
	retries  uint
	group    singleflight.Group
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: _defaultMaxIdleConnPerHost,
		},
		Timeout: _defaultTimeout,
	}
}

// NewManager returns a new instance of Manager.
func NewManager(rawURL string, opts ...Option) (*Manager, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("endpoint must have http:// or https:// scheme, got: %s", rawURL)
	}
	urlTemplate := fasttemplate.New(rawURL, "{{", "}}")

	mng := &Manager{
		url: urlTemplate,

		cache:    NewTTLCache(_defaultTTL),
		client:   defaultHTTPClient(),
		useCache: true,
		retries:  _defaultRetries,
	}

	for _, opt := range opts {
		opt(mng)
	}

	if mng.retries == 0 {
		return nil, ErrInvalidNumRetries
	}

	return mng, nil
}

// FetchKey fetches JWKS from public source or cache.
//
// The cache and singleflight keys are scoped to the resolved JWKS endpoint URL,
// not only the JWT header kid. This prevents a key cached from one trust domain
// (e.g., tenant A's templated JWKS URL) from satisfying a verification request
// for a different trust domain (tenant B) that happens to advertise the same kid.
// JWT kid values are not globally unique by spec — common operational labels like
// "default" or rotation IDs may collide across issuers.
func (m *Manager) FetchKey(ctx context.Context, kid string, tokenVars map[string]any) (*JWK, error) {
	if kid == "" {
		return nil, ErrKeyIDNotProvided
	}

	jwkURL, err := m.resolveURL(tokenVars)
	if err != nil {
		return nil, err
	}

	cacheKey := cacheKey(jwkURL, kid)

	// If useCache is true, first try to get key from cache.
	if m.useCache {
		key, err := m.cache.Get(cacheKey)
		if err == nil {
			return key, nil
		}
	}

	// Otherwise fetch from public JWKS.
	v, err, _ := m.group.Do(cacheKey, func() (any, error) {
		return m.fetchKey(ctx, jwkURL, kid)
	})
	if err != nil {
		return nil, err
	}

	return v.(*JWK), nil
}

// cacheKey builds a cache/singleflight key namespaced by the resolved JWKS URL.
// The NUL byte is a delimiter that cannot appear in a valid URL or JWT kid, so
// the encoding is unambiguous.
func cacheKey(resolvedURL, kid string) string {
	return resolvedURL + "\x00" + kid
}

func (m *Manager) loadData(req *http.Request) ([]byte, error) {
	resp, err := m.client.Do(req) //nolint:gosec // URL is from server configuration, not user input.
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errUnexpectedStatusCode, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// resolveURL applies tokenVars to the URL template and rejects results with
// path traversal sequences. Runs on every FetchKey call (including cache hits)
// so that cache lookups are scoped to the validated, fully-resolved URL.
func (m *Manager) resolveURL(tokenVars map[string]any) (string, error) {
	jwkURL := m.url.ExecuteString(tokenVars)

	u, err := url.Parse(jwkURL)
	if err != nil {
		return "", fmt.Errorf("error parsing JWKS URL: %w", err)
	}
	if u.Path != "" {
		if cleanPath := path.Clean(u.Path); cleanPath != u.Path {
			log.Info().Str("path", u.Path).Str("clean_path", cleanPath).
				Msg("JWKS URL path contains traversal sequences, request rejected")
			return "", fmt.Errorf("JWKS URL path contains traversal sequences: %q", u.Path)
		}
	}
	return jwkURL, nil
}

func (m *Manager) fetchKey(ctx context.Context, jwkURL, kid string) (*JWK, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwkURL, nil)
	if err != nil {
		return nil, err
	}

	var set jwk.KeySpecSet
	var data []byte
	var lastError error

	retries := m.retries
	for {
		if retries == 0 {
			return nil, lastError
		}
		retries--
		var err error
		data, err = m.loadData(req)
		if err != nil {
			lastError = err
			continue
		}
		break
	}

	if err := json.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("%w: %v", errUnmarshal, err)
	}

	if len(set.Keys) == 0 {
		return nil, ErrPublicKeyNotFound
	}

	var res *JWK

	// Save new set into cache.
	for _, spec := range set.Keys {
		key, err := spec.ToJWK()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errConvert, err)
		}

		if key.Use != "sig" {
			// Not interested in other types of Use in Centrifugo.
			continue
		}

		if m.useCache {
			_ = m.cache.Add(cacheKey(jwkURL, key.Kid), key)
		}

		if key.Kid == kid {
			res = key
		}
	}

	if res == nil {
		return nil, ErrPublicKeyNotFound
	}

	return res, nil
}
