package jwks

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/rakutentech/jwk-go/jwk"
	"golang.org/x/sync/singleflight"
)

const (
	_defaultRetries = 2
	_defaultTimeout = 1 * time.Second
	_defaultTTL     = 1 * time.Hour
)

// JWK represents an unparsed JSON Web Key (JWK) in its wire format.
type JWK = jwk.JWK

var (
	// ErrConnectionFailed raises when JWKS endpoint cannot be reached.
	ErrConnectionFailed = errors.New("jwks: connection failed")
	// ErrInvalidURL raises when input url has invalid format.
	ErrInvalidURL = errors.New("jwks: invalid url value or format")
	// ErrKeyIDNotProvided raises when input kid is not present.
	ErrKeyIDNotProvided = errors.New("jwks: kid is not provided")
	// ErrPublicKeyNotFound raises when no public key is found.
	ErrPublicKeyNotFound = errors.New("jwks: public key not found")
)

// Manager fetches and returns JWK from public source.
type Manager struct {
	url     *url.URL
	cache   Cache
	client  *http.Client
	lookup  bool
	retries int
	group   singleflight.Group
}

// NewManager returns a new instance of `Manager`.
func NewManager(rawurl string, opts ...Option) (*Manager, error) {
	url, err := url.Parse(rawurl)
	if err != nil {
		return nil, ErrInvalidURL
	}

	mng := &Manager{
		url:     url,
		cache:   NewTTLCache(_defaultTTL),
		client:  &http.Client{Timeout: _defaultTimeout},
		lookup:  true,
		retries: _defaultRetries,
	}

	for _, opt := range opts {
		opt(mng)
	}

	return mng, nil
}

// FetchKey fetches JWKS from public source or cache.
func (m *Manager) FetchKey(ctx context.Context, kid string) (*JWK, error) {
	if kid == "" {
		return nil, ErrKeyIDNotProvided
	}

	// If lookup is true, first try to get key from cache.
	if m.lookup {
		key, err := m.cache.Get(ctx, kid)
		if err == nil {
			return key, nil
		}
	}

	// Otherwise fetch from public JWKS.
	v, err, _ := m.group.Do(kid, func() (interface{}, error) {
		return m.fetchKey(ctx, kid)
	})
	if err != nil {
		return nil, err
	}

	return v.(*JWK), nil
}

func (m *Manager) fetchKey(ctx context.Context, kid string) (*JWK, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.url.String(), nil)
	if err != nil {
		return nil, err
	}

	var set jwk.KeySpecSet

	// Make sure that you have exponential back off on this http request with retries.
	retries := m.retries
	for retries > 0 {
		retries--

		resp, err := m.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		if err := json.Unmarshal(data, &set); err != nil {
			return nil, err
		}

		break
	}

	if retries == 0 {
		return nil, ErrConnectionFailed
	}

	if len(set.Keys) == 0 {
		return nil, ErrPublicKeyNotFound
	}

	var res *JWK

	// Save new set into cache.
	for _, spec := range set.Keys {
		jwk, err := spec.ToJWK()
		if err != nil {
			return nil, err
		}

		if m.lookup {
			if err := m.cache.Add(ctx, jwk); err != nil {
				return nil, err
			}
		}

		if jwk.Kid == kid {
			res = jwk
		}
	}

	if res == nil || res.Kty != "RSA" || res.Use != "sig" {
		return nil, ErrPublicKeyNotFound
	}

	return res, nil
}
