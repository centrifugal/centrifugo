package jwks

import (
	"errors"
)

var (
	// ErrCacheNotFound returned when cache value not found.
	ErrCacheNotFound = errors.New("cache: value not found")
)

// Cache works with cache layer.
type Cache interface {
	Add(key *JWK) error
	Get(kid string) (*JWK, error)
	Len() (int, error)
}
