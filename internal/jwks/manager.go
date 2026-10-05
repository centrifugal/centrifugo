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
	"sync"
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
	// _defaultStaleRetryInterval is how often an expired key is fetched again
	// while the JWKS endpoint can't be reached and the key keeps being used.
	_defaultStaleRetryInterval = 1 * time.Minute
	// _defaultRefetchInterval is the minimal interval between fetches of a JWKS
	// endpoint caused by keys missing in the cache. Tokens are not verified yet
	// when their key is looked up, so without it every token with a new random
	// kid would make a request to the JWKS endpoint. It must stay well below the
	// cache TTL: a key missing in the cache within it after a fetch is a key the
	// endpoint did not return.
	_defaultRefetchInterval = 5 * time.Second
	// _maxTrackedEndpoints bounds how many resolved JWKS URLs remember their last
	// fetch time. URL templates are filled from token claims, which are not
	// verified at this point either.
	_maxTrackedEndpoints = 1024
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

	staleRetryInterval time.Duration
	refetchInterval    time.Duration

	fetchedMu sync.Mutex
	fetchedAt map[string]time.Time // Resolved JWKS URL -> last successful fetch.
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

		staleRetryInterval: _defaultStaleRetryInterval,
		refetchInterval:    _defaultRefetchInterval,
		fetchedAt:          make(map[string]time.Time),
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
		// The endpoint was fetched a moment ago and did not return this kid.
		// Fetching it again for every such token would let anyone send requests
		// to the JWKS endpoint, so the kid is unknown until the interval passes.
		// The interval is counted from the last fetch only, so it can't be
		// extended: a key the endpoint starts to return is picked up at most one
		// interval later.
		if m.fetchedRecently(jwkURL) {
			// A concurrent fetch may have completed after the lookup above. It
			// updates the cache before recording the fetch time, so look again.
			if key, err := m.cache.Get(cacheKey); err == nil {
				return key, nil
			}
			return nil, ErrPublicKeyNotFound
		}
	}

	// Otherwise fetch from public JWKS. Lookups of different kids share one
	// fetch of the endpoint: it returns the whole key set.
	v, err, _ := m.group.Do(jwkURL, func() (any, error) {
		return m.fetchKeys(ctx, jwkURL)
	})
	if err == nil {
		key, ok := v.(map[string]*JWK)[kid]
		if !ok {
			return nil, ErrPublicKeyNotFound
		}
		return key, nil
	}
	// Keys expire from the cache to pick up keys removed from the JWKS
	// endpoint. When the endpoint can't tell, as it can't be reached or
	// returns an error, keep using the key fetched before: an unavailable
	// endpoint must not fail verification of the tokens it issued.
	if m.useCache {
		if key, staleErr := m.cache.GetStale(cacheKey, m.staleRetryInterval); staleErr == nil {
			log.Warn().Err(err).Str("kid", kid).Msg("error fetching JWKS, using previously fetched key")
			return key, nil
		}
	}
	return nil, err
}

// fetchedRecently reports whether the endpoint was fetched successfully within
// the refetch interval.
func (m *Manager) fetchedRecently(jwkURL string) bool {
	m.fetchedMu.Lock()
	defer m.fetchedMu.Unlock()
	at, ok := m.fetchedAt[jwkURL]
	return ok && time.Since(at) < m.refetchInterval
}

// markFetched remembers a successful fetch of the endpoint.
func (m *Manager) markFetched(jwkURL string) {
	now := time.Now()
	m.fetchedMu.Lock()
	defer m.fetchedMu.Unlock()
	if len(m.fetchedAt) >= _maxTrackedEndpoints {
		// Entries past the interval have no effect, drop them. If all are
		// recent, forget them all: that only allows extra fetches.
		for u, at := range m.fetchedAt {
			if now.Sub(at) >= m.refetchInterval {
				delete(m.fetchedAt, u)
			}
		}
		if len(m.fetchedAt) >= _maxTrackedEndpoints {
			clear(m.fetchedAt)
		}
	}
	m.fetchedAt[jwkURL] = now
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

// fetchKeys fetches the key set of the endpoint, keyed by kid, and replaces
// the cached keys of the endpoint with it.
func (m *Manager) fetchKeys(ctx context.Context, jwkURL string) (map[string]*JWK, error) {
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

	keys := make(map[string]*JWK, len(set.Keys))
	cached := make(map[string]*JWK, len(set.Keys))
	for _, spec := range set.Keys {
		key, err := spec.ToJWK()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errConvert, err)
		}

		if key.Use != "sig" {
			// Not interested in other types of Use in Centrifugo.
			continue
		}

		keys[key.Kid] = key
		cached[cacheKey(jwkURL, key.Kid)] = key
	}

	// Save new set into cache. Keys the endpoint no longer returns are
	// removed, so they are not used when it can't be reached later.
	if m.useCache {
		if err := m.cache.ReplacePrefix(cacheKey(jwkURL, ""), cached); err == nil {
			m.markFetched(jwkURL)
		}
	}

	return keys, nil
}
