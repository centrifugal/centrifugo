package jwks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rakutentech/jwk-go/jwk"
	"github.com/stretchr/testify/require"
)

type testKey struct {
	Kid string
	Key any
}

func parseRSA(t *testing.T, k *JWK) *rsa.PublicKey {
	t.Helper()
	spec, err := k.ParseKeySpec()
	require.NoError(t, err)
	pub, ok := spec.Key.(*rsa.PublicKey)
	require.True(t, ok)
	return pub
}

func randomKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, nil, err
	}

	return privateKey, &privateKey.PublicKey, nil
}

func jwksHandler(keys ...testKey) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		specs := jwk.KeySpecSet{}

		for _, key := range keys {
			spec := jwk.NewSpecWithID(key.Kid, key.Key)
			spec.Use = "sig"
			specs.Keys = append(specs.Keys, *spec)
		}

		data, err := json.Marshal(specs)
		if err != nil {
			http.Error(w, "Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
}

func TestManagerFetchKey_UnmarshalError(t *testing.T) {
	mux := http.NewServeMux()
	path := "/.well-known/jwks.json"
	mux.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`...`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	manager, err := NewManager(server.URL + path)
	require.NoError(t, err)

	_, err = manager.FetchKey(context.Background(), "202101", nil)
	require.ErrorIs(t, err, errUnmarshal)
}

func TestManagerFetchKey_KeyNotFound(t *testing.T) {
	mux := http.NewServeMux()
	path := "/.well-known/jwks.json"
	mux.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"keys": []}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	manager, err := NewManager(server.URL + path)
	require.NoError(t, err)

	_, err = manager.FetchKey(context.Background(), "202101", nil)
	require.ErrorIs(t, err, ErrPublicKeyNotFound)
}

func TestManagerFetchKey_WrongStatusCode(t *testing.T) {
	mux := http.NewServeMux()
	path := "/.well-known/jwks.json"
	mux.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	manager, err := NewManager(server.URL + path)
	require.NoError(t, err)

	_, err = manager.FetchKey(context.Background(), "202101", nil)
	require.ErrorIs(t, err, errUnexpectedStatusCode)
}

func TestManagerInitialFetchKey(t *testing.T) {
	_, pubKey, err := randomKeys()
	require.NoError(t, err)

	testCases := []struct {
		Name    string
		Handler http.Handler
		Kid     string
		Error   error
	}{
		{
			Name:    "OK",
			Handler: jwksHandler(testKey{"202101", pubKey}),
			Kid:     "202101",
		},
		{
			Name:    "NotFound",
			Handler: jwksHandler(testKey{"202101", pubKey}),
			Kid:     "202102",
			Error:   ErrPublicKeyNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			r := require.New(t)

			ts := httptest.NewServer(tc.Handler)
			defer ts.Close()

			manager, err := NewManager(ts.URL)
			r.NoError(err)

			key, err := manager.FetchKey(context.Background(), tc.Kid, nil)
			if tc.Error != nil {
				r.Error(err)
				r.ErrorIs(err, tc.Error)
			} else {
				r.NoError(err)
				r.Equal(tc.Kid, key.Kid)
			}
		})
	}
}

func TestManagerFetchKey_PathTraversalRejected(t *testing.T) {
	kid := "test-kid"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request should have been rejected before reaching the server")
	}))
	defer ts.Close()

	manager, err := NewManager(ts.URL + "/tenants/{{tenant}}/jwks.json")
	require.NoError(t, err)

	// Path traversal via ".." — must be rejected.
	tokenVars := map[string]any{"tenant": "../../etc"}
	_, err = manager.FetchKey(context.Background(), kid, tokenVars)
	require.Error(t, err)
	require.Contains(t, err.Error(), "traversal")

	// Single ".." segment — also rejected.
	tokenVars = map[string]any{"tenant": ".."}
	_, err = manager.FetchKey(context.Background(), kid, tokenVars)
	require.Error(t, err)
	require.Contains(t, err.Error(), "traversal")

	// Clean value — must succeed (uses real server).
	_, pubKey, err := randomKeys()
	require.NoError(t, err)

	ts2 := httptest.NewServer(jwksHandler(testKey{kid, pubKey}))
	defer ts2.Close()

	manager, err = NewManager(ts2.URL + "/tenants/{{tenant}}/jwks.json")
	require.NoError(t, err)

	tokenVars = map[string]any{"tenant": "acme"}
	key, err := manager.FetchKey(context.Background(), kid, tokenVars)
	require.NoError(t, err)
	require.Equal(t, kid, key.Kid)
}

func TestManagerCachedFetchKey(t *testing.T) {
	testCases := []struct {
		Name         string
		Options      []Option
		ExpectedSize int
	}{
		{
			Name:         "Default",
			ExpectedSize: 1,
		},
		{
			Name:         "NoCacheLookup",
			Options:      []Option{WithUseCache(false)},
			ExpectedSize: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			r := require.New(t)

			ctx := context.Background()
			kid := "202101"

			_, pubKey, err := randomKeys()
			r.NoError(err)

			ts := httptest.NewServer(jwksHandler(testKey{kid, pubKey}))
			defer ts.Close()

			manager, err := NewManager(ts.URL, tc.Options...)
			r.NoError(err)

			key, err := manager.FetchKey(ctx, kid, nil)
			r.NoError(err)
			r.Equal(kid, key.Kid)

			size, err := manager.cache.Len()
			r.NoError(err)
			r.Equal(tc.ExpectedSize, size)
		})
	}
}

// TestManagerFetchKey_CacheScopedByResolvedURL is a regression test for a
// cross-trust-domain cache reuse bug: when JWKS URLs are templated from token
// claims (e.g., per-tenant endpoints), a key cached from tenant A's endpoint
// must NOT satisfy a lookup for tenant B's endpoint, even if both JWKS documents
// advertise the same kid. kid values are not globally unique by spec and may
// collide across issuers.
func TestManagerFetchKey_CacheScopedByResolvedURL(t *testing.T) {
	const sharedKid = "shared-kid"

	_, tenantAPubKey, err := randomKeys()
	require.NoError(t, err)
	_, tenantBPubKey, err := randomKeys()
	require.NoError(t, err)

	var tenantARequests, tenantBRequests int32

	mux := http.NewServeMux()
	mux.HandleFunc("/tenant-a/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tenantARequests, 1)
		jwksHandler(testKey{sharedKid, tenantAPubKey}).ServeHTTP(w, r)
	})
	mux.HandleFunc("/tenant-b/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tenantBRequests, 1)
		jwksHandler(testKey{sharedKid, tenantBPubKey}).ServeHTTP(w, r)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	manager, err := NewManager(ts.URL + "/{{tenant}}/jwks.json")
	require.NoError(t, err)

	ctx := context.Background()

	keyA, err := manager.FetchKey(ctx, sharedKid, map[string]any{"tenant": "tenant-a"})
	require.NoError(t, err)
	require.Equal(t, tenantAPubKey.N, parseRSA(t, keyA).N)
	require.Equal(t, int32(1), atomic.LoadInt32(&tenantARequests))
	require.Equal(t, int32(0), atomic.LoadInt32(&tenantBRequests))

	// Fetching for tenant B with the same kid must consult tenant B's endpoint,
	// not reuse the cached tenant A key.
	keyB, err := manager.FetchKey(ctx, sharedKid, map[string]any{"tenant": "tenant-b"})
	require.NoError(t, err)
	require.Equal(t, tenantBPubKey.N, parseRSA(t, keyB).N)
	require.Equal(t, int32(1), atomic.LoadInt32(&tenantARequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&tenantBRequests))

	// Second fetch for tenant A should hit the cache — no new HTTP request.
	_, err = manager.FetchKey(ctx, sharedKid, map[string]any{"tenant": "tenant-a"})
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&tenantARequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&tenantBRequests))
}

// rotatingJWKSServer serves the keys set with setKeys, or fails with 500 after
// setDown.
type rotatingJWKSServer struct {
	*httptest.Server
	handler  atomic.Value // http.Handler
	requests atomic.Int32
}

func newRotatingJWKSServer(t *testing.T, keys ...testKey) *rotatingJWKSServer {
	s := &rotatingJWKSServer{}
	s.setKeys(keys...)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		s.handler.Load().(http.Handler).ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *rotatingJWKSServer) setKeys(keys ...testKey) {
	s.handler.Store(jwksHandler(keys...))
}

func (s *rotatingJWKSServer) setDown() {
	s.handler.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
}

func newRotationTestManager(t *testing.T, url string, ttl time.Duration) *Manager {
	cache := NewTTLCache(ttl)
	t.Cleanup(func() { _ = cache.Stop() })
	manager, err := NewManager(url, WithCache(cache))
	require.NoError(t, err)
	return manager
}

// A key removed from the JWKS endpoint stops being trusted once its cache TTL
// passes, however often it is used.
func TestManagerFetchKey_RemovedKeyExpires(t *testing.T) {
	const ttl = 200 * time.Millisecond
	_, pubKey, err := randomKeys()
	require.NoError(t, err)
	_, newPubKey, err := randomKeys()
	require.NoError(t, err)

	ts := newRotatingJWKSServer(t, testKey{"old", pubKey})
	manager := newRotationTestManager(t, ts.URL, ttl)
	ctx := context.Background()

	_, err = manager.FetchKey(ctx, "old", nil)
	require.NoError(t, err)

	// Using it far more often than the TTL does not keep it.
	ts.setKeys(testKey{"new", newPubKey})
	require.Eventually(t, func() bool {
		_, err := manager.FetchKey(ctx, "old", nil)
		return errors.Is(err, ErrPublicKeyNotFound)
	}, 10*ttl, ttl/20)

	key, err := manager.FetchKey(ctx, "new", nil)
	require.NoError(t, err)
	require.Equal(t, newPubKey.N, parseRSA(t, key).N)
}

// While the JWKS endpoint can't be reached, an expired key keeps being used,
// and is fetched again at most once per stale retry interval.
func TestManagerFetchKey_EndpointDownUsesFetchedKey(t *testing.T) {
	const ttl = 100 * time.Millisecond
	_, pubKey, err := randomKeys()
	require.NoError(t, err)

	ts := newRotatingJWKSServer(t, testKey{"kid", pubKey})
	manager := newRotationTestManager(t, ts.URL, ttl)
	manager.staleRetryInterval = time.Minute
	ctx := context.Background()

	_, err = manager.FetchKey(ctx, "kid", nil)
	require.NoError(t, err)

	// Keeps being used when fetching it again after the TTL fails.
	ts.setDown()
	require.Eventually(t, func() bool {
		key, err := manager.FetchKey(ctx, "kid", nil)
		if err != nil || key.Kid != "kid" {
			t.Errorf("unexpected key or error: %v", err)
			return true
		}
		return ts.requests.Load() > 1
	}, 10*ttl, ttl/20)
	for i := 0; i < 10; i++ {
		key, err := manager.FetchKey(ctx, "kid", nil)
		require.NoError(t, err)
		require.Equal(t, pubKey.N, parseRSA(t, key).N)
	}
	// The initial fetch, then one failed fetch with its retries.
	require.Equal(t, int32(1+_defaultRetries), ts.requests.Load())
}

// A key removed from the JWKS endpoint is not used when the endpoint can't be
// reached later.
func TestManagerFetchKey_EndpointDownDoesNotUseRemovedKey(t *testing.T) {
	const ttl = 100 * time.Millisecond
	_, pubKey1, err := randomKeys()
	require.NoError(t, err)
	_, pubKey2, err := randomKeys()
	require.NoError(t, err)

	ts := newRotatingJWKSServer(t, testKey{"1", pubKey1}, testKey{"2", pubKey2})
	manager := newRotationTestManager(t, ts.URL, ttl)
	ctx := context.Background()

	_, err = manager.FetchKey(ctx, "1", nil)
	require.NoError(t, err)

	// Key 2 is removed, which the fetch of expired key 1 learns.
	ts.setKeys(testKey{"1", pubKey1})
	require.Eventually(t, func() bool {
		if _, err := manager.FetchKey(ctx, "1", nil); err != nil {
			t.Errorf("unexpected error: %v", err)
			return true
		}
		return ts.requests.Load() > 1
	}, 10*ttl, ttl/20)

	ts.setDown()
	_, err = manager.FetchKey(ctx, "2", nil)
	require.ErrorIs(t, err, errUnexpectedStatusCode)
}

// Without the cache, a failed fetch fails the lookup.
func TestManagerFetchKey_EndpointDownNoCache(t *testing.T) {
	_, pubKey, err := randomKeys()
	require.NoError(t, err)

	ts := newRotatingJWKSServer(t, testKey{"kid", pubKey})
	manager, err := NewManager(ts.URL, WithUseCache(false))
	require.NoError(t, err)
	ctx := context.Background()

	_, err = manager.FetchKey(ctx, "kid", nil)
	require.NoError(t, err)
	ts.setDown()
	_, err = manager.FetchKey(ctx, "kid", nil)
	require.ErrorIs(t, err, errUnexpectedStatusCode)
}
