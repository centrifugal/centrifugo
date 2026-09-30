package jwks

import (
	"errors"
	"time"
)

var (
	// ErrCacheNotFound returned when cache value not found.
	ErrCacheNotFound = errors.New("cache: value not found")
)

// Cache works with cache layer. The cacheKey is opaque to the cache and chosen by
// the caller — Manager uses a composite of the resolved JWKS endpoint URL and the
// JWT header kid so that keys cached from one trust domain (templated URL) cannot
// satisfy lookups for another. See Manager.FetchKey.
type Cache interface {
	Add(cacheKey string, key *JWK) error
	// ReplacePrefix replaces the keys under the cache keys starting with
	// prefix, removing those missing from keys.
	ReplacePrefix(prefix string, keys map[string]*JWK) error
	Get(cacheKey string) (*JWK, error)
	// GetStale gets the key even if it expired, and lets Get find it for
	// retryAfter more.
	GetStale(cacheKey string, retryAfter time.Duration) (*JWK, error)
	Len() (int, error)
}
