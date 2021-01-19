package jwks

import (
	"context"
	"errors"
)

var (
	// ErrEmptyKeyID raises when input kid is empty.
	ErrEmptyKeyID = errors.New("cache: empty kid")
	// ErrCacheNotFound raises when cache value not found.
	ErrCacheNotFound = errors.New("cache: value not found")
	// ErrInvalidValue raises when type conversion to JWK has been failed.
	ErrInvalidValue = errors.New("cache: invalid value")
)

// Cache works with cache layer.
type Cache interface {
	Add(ctx context.Context, key *JWK) error
	Get(ctx context.Context, kid string) (*JWK, error)
	Remove(ctx context.Context, kid string) error
	Contains(ctx context.Context, kid string) (bool, error)
	Len(ctx context.Context) (int, error)
	Purge(ctx context.Context) error
}
