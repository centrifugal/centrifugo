package jwks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTTLCacheInit(t *testing.T) {
	cache := NewTTLCache(5 * time.Minute)
	require.NotNil(t, cache)
}

func TestTTLCacheAdd(t *testing.T) {
	testCases := []struct {
		Name string
		TTL  time.Duration
		Ops  int
	}{
		{
			Name: "OK",
			TTL:  5 * time.Second,
			Ops:  100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()

			cache := NewTTLCache(tc.TTL)
			require.NotNil(t, cache)

			for i := 0; i < tc.Ops; i++ {
				require.NoError(t, cache.Add(ctx, &JWK{
					Kid: fmt.Sprintf("key-%d", i+1),
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
				}))
			}
		})
	}
}

func TestTTLCacheGet(t *testing.T) {
	testCases := []struct {
		Name  string
		Key   *JWK
		Kid   string
		Error error
	}{
		{
			Name: "OK",
			Key: &JWK{
				Kid: "202101",
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
			},
			Kid: "202101",
		},
		{
			Name: "NotFound",
			Key: &JWK{
				Kid: "202101",
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
			},
			Kid:   "202102",
			Error: ErrCacheNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()

			cache := NewTTLCache(5 * time.Minute)
			require.NotNil(t, cache)
			require.NoError(t, cache.Add(ctx, tc.Key))

			key, err := cache.Get(ctx, tc.Kid)
			if tc.Error != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.Error)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, tc.Key, key)
			}
		})
	}
}

func TestTTLCacheRemove(t *testing.T) {
	testCases := []struct {
		Name string
		Adds int
		Dels int
		Len  int
	}{
		{
			Name: "OK",
			Adds: 75,
			Dels: 50,
			Len:  25,
		},
		{
			Name: "RemoveUntilEmpty",
			Adds: 75,
			Dels: 100,
			Len:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()

			cache := NewTTLCache(5 * time.Minute)
			require.NotNil(t, cache)

			for i := 0; i < tc.Adds; i++ {
				require.NoError(t, cache.Add(ctx, &JWK{
					Kid: fmt.Sprintf("key-%d", i+1),
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
				}))
			}

			for i := 0; i < tc.Dels; i++ {
				kid := fmt.Sprintf("key-%d", i+1)
				require.NoError(t, cache.Remove(ctx, kid))
			}

			n, err := cache.Len(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.Len, n)
		})
	}
}

func TestTTLCacheContains(t *testing.T) {
	testCases := []struct {
		Name  string
		Key   *JWK
		Kid   string
		Found bool
	}{
		{
			Name: "OK",
			Key: &JWK{
				Kid: "202101",
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
			},
			Kid:   "202101",
			Found: true,
		},
		{
			Name: "NotFound",
			Key: &JWK{
				Kid: "202101",
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
			},
			Kid:   "202102",
			Found: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()

			cache := NewTTLCache(5 * time.Minute)
			require.NotNil(t, cache)
			require.NoError(t, cache.Add(ctx, tc.Key))

			found, err := cache.Contains(ctx, tc.Kid)
			require.NoError(t, err)

			require.Equal(t, tc.Found, found)
		})
	}
}

func TestTTLCacheLen(t *testing.T) {
	testCases := []struct {
		Name string
		Ops  int
		Len  int
	}{
		{
			Name: "OK",
			Ops:  50,
			Len:  50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()

			cache := NewTTLCache(5 * time.Second)
			require.NotNil(t, cache)

			for i := 0; i < tc.Ops; i++ {
				require.NoError(t, cache.Add(ctx, &JWK{
					Kid: fmt.Sprintf("key-%d", i+1),
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
				}))
			}

			n, err := cache.Len(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.Len, n)
		})
	}
}

func TestTTLCachePurge(t *testing.T) {
	testCases := []struct {
		Name string
		Ops  int
	}{
		{
			Name: "OK",
			Ops:  50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()

			cache := NewTTLCache(5 * time.Second)
			require.NotNil(t, cache)

			for i := 0; i < tc.Ops; i++ {
				require.NoError(t, cache.Add(ctx, &JWK{
					Kid: fmt.Sprintf("key-%d", i+1),
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
				}))
			}

			require.NoError(t, cache.Purge(ctx))

			n, err := cache.Len(ctx)
			require.NoError(t, err)
			require.Equal(t, 0, n)
		})
	}
}

func TestTTLCacheCleanup(t *testing.T) {
	ctx := context.Background()
	cache := NewTTLCache(1 * time.Millisecond)

	for i := 0; i < 10; i++ {
		require.NoError(t, cache.Add(ctx, &JWK{
			Kid: fmt.Sprintf("key-%d", i+1),
			Kty: "RSA",
			Alg: "RS256",
			Use: "sig",
		}))
	}

	time.Sleep(2 * time.Second)

	n, err := cache.Len(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
