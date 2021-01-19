package jwks

import (
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
				require.NoError(t, cache.Add(&JWK{
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
			cache := NewTTLCache(5 * time.Minute)
			require.NotNil(t, cache)
			require.NoError(t, cache.Add(tc.Key))

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
				require.NoError(t, cache.Add(&JWK{
					Kid: fmt.Sprintf("key-%d", i+1),
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
		require.NoError(t, cache.Add(&JWK{
			Kid: fmt.Sprintf("key-%d", i+1),
			Kty: "RSA",
			Alg: "RS256",
			Use: "sig",
		}))
	}

	time.Sleep(2 * time.Second)

	n, err := cache.Len()
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
