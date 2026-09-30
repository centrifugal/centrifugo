package jwks

import (
	"errors"
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
			cache := NewTTLCache(tc.TTL)
			require.NotNil(t, cache)

			for i := 0; i < tc.Ops; i++ {
				kid := fmt.Sprintf("key-%d", i+1)
				require.NoError(t, cache.Add(kid, &JWK{
					Kid: kid,
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
			cache := NewTTLCache(5 * time.Minute)
			require.NotNil(t, cache)
			require.NoError(t, cache.Add(tc.Key.Kid, tc.Key))

			key, err := cache.Get(tc.Kid)
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
		Name      string
		NumAdd    int
		NumDelete int
		Len       int
	}{
		{
			Name:      "OK",
			NumAdd:    75,
			NumDelete: 50,
			Len:       25,
		},
		{
			Name:      "RemoveUntilEmpty",
			NumAdd:    75,
			NumDelete: 100,
			Len:       0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			cache := NewTTLCache(5 * time.Minute)
			require.NotNil(t, cache)

			for i := 0; i < tc.NumAdd; i++ {
				kid := fmt.Sprintf("key-%d", i+1)
				require.NoError(t, cache.Add(kid, &JWK{
					Kid: kid,
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
				}))
			}

			for i := 0; i < tc.NumDelete; i++ {
				kid := fmt.Sprintf("key-%d", i+1)
				require.NoError(t, cache.remove(kid))
			}

			n, err := cache.Len()
			require.NoError(t, err)
			require.Equal(t, tc.Len, n)
		})
	}
}

func TestTTLCacheCleanup(t *testing.T) {
	cache := NewTTLCache(1 * time.Millisecond)

	for i := 0; i < 10; i++ {
		kid := fmt.Sprintf("key-%d", i+1)
		require.NoError(t, cache.Add(kid, &JWK{
			Kid: kid,
			Kty: "RSA",
			Alg: "RS256",
			Use: "sig",
		}))
	}

	require.Eventually(t, func() bool {
		n, err := cache.Len()
		return err == nil && n == 0
	}, 5*time.Second, 10*time.Millisecond)
}

// An item expires the TTL after it was added, however often it is got in
// between: a key rotated out of the JWKS endpoint must be re-fetched.
func TestTTLCacheGetDoesNotExtendTTL(t *testing.T) {
	const ttl = 100 * time.Millisecond
	cache := NewTTLCache(ttl)
	t.Cleanup(func() { _ = cache.Stop() })
	require.NoError(t, cache.Add("kid", &JWK{Kid: "kid"}))

	// Getting it far more often than the TTL does not keep it.
	require.Eventually(t, func() bool {
		_, err := cache.Get("kid")
		return errors.Is(err, ErrCacheNotFound)
	}, 10*ttl, ttl/20)
}

// GetStale gets an expired item, and lets Get find it for retryAfter more.
func TestTTLCacheGetStale(t *testing.T) {
	const ttl = 100 * time.Millisecond
	cache := NewTTLCache(ttl)
	t.Cleanup(func() { _ = cache.Stop() })

	_, err := cache.GetStale("kid", time.Minute)
	require.ErrorIs(t, err, ErrCacheNotFound)

	require.NoError(t, cache.Add("kid", &JWK{Kid: "kid"}))
	require.Eventually(t, func() bool {
		_, err := cache.Get("kid")
		return errors.Is(err, ErrCacheNotFound)
	}, 10*ttl, ttl/20)

	key, err := cache.GetStale("kid", time.Minute)
	require.NoError(t, err)
	require.Equal(t, "kid", key.Kid)
	key, err = cache.Get("kid")
	require.NoError(t, err)
	require.Equal(t, "kid", key.Kid)
}

// Cleanup removes expired items unless they were used within the TTL, so a
// key still in use stays for GetStale to fall back on.
func TestTTLCacheCleanupKeepsUsedItems(t *testing.T) {
	const ttl = 100 * time.Millisecond
	cache := NewTTLCache(ttl)
	t.Cleanup(func() { _ = cache.Stop() })
	require.NoError(t, cache.Add("used", &JWK{Kid: "used"}))
	require.NoError(t, cache.Add("unused", &JWK{Kid: "unused"}))

	require.Eventually(t, func() bool {
		if _, err := cache.GetStale("used", 0); err != nil {
			t.Errorf("unexpected error: %v", err)
			return true
		}
		cache.cleanup()
		return !cache.has("unused")
	}, 10*ttl, ttl/20)
	require.True(t, cache.has("used"))
}

func (tc *TTLCache) has(cacheKey string) bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	_, ok := tc.items[cacheKey]
	return ok
}

// ReplacePrefix replaces the items under the prefix only.
func TestTTLCacheReplacePrefix(t *testing.T) {
	cache := NewTTLCache(time.Minute)
	t.Cleanup(func() { _ = cache.Stop() })
	require.NoError(t, cache.Add("a/1", &JWK{Kid: "1"}))
	require.NoError(t, cache.Add("a/2", &JWK{Kid: "2"}))
	require.NoError(t, cache.Add("b/1", &JWK{Kid: "1"}))

	require.NoError(t, cache.ReplacePrefix("a/", map[string]*JWK{"a/2": {Kid: "2"}, "a/3": {Kid: "3"}}))

	_, err := cache.Get("a/1")
	require.ErrorIs(t, err, ErrCacheNotFound)
	for _, cacheKey := range []string{"a/2", "a/3", "b/1"} {
		_, err := cache.Get(cacheKey)
		require.NoError(t, err, cacheKey)
	}
}

// Every JWKS token verification goes through Get, so a cache hit must not
// allocate.
func TestTTLCacheGetDoesNotAllocate(t *testing.T) {
	cache := NewTTLCache(time.Minute)
	t.Cleanup(func() { _ = cache.Stop() })
	require.NoError(t, cache.Add("kid", &JWK{Kid: "kid"}))

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := cache.Get("kid"); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocs)
}

// Concurrent token verifications with the same key id get the same item.
func BenchmarkTTLCacheGetParallel(b *testing.B) {
	cache := NewTTLCache(time.Minute)
	b.Cleanup(func() { _ = cache.Stop() })
	require.NoError(b, cache.Add("kid", &JWK{Kid: "kid"}))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := cache.Get("kid"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
