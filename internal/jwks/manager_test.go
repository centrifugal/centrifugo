package jwks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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

var (
	testKeysMu sync.Mutex
	testKeys   = map[int]*rsa.PrivateKey{}
)

// randomKeys returns the n-th test RSA key pair. RSA key generation is slow
// (especially with -race), so keys are generated once and shared by tests.
// Tests needing several distinct keys use different n.
func randomKeys(n int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	testKeysMu.Lock()
	defer testKeysMu.Unlock()
	privateKey, ok := testKeys[n]
	if !ok {
		var err error
		privateKey, err = rsa.GenerateKey(rand.Reader, 1024)
		if err != nil {
			return nil, nil, err
		}
		testKeys[n] = privateKey
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
	_, pubKey, err := randomKeys(0)
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
	_, pubKey, err := randomKeys(0)
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

			_, pubKey, err := randomKeys(0)
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

	_, tenantAPubKey, err := randomKeys(0)
	require.NoError(t, err)
	_, tenantBPubKey, err := randomKeys(1)
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
	// These tests use a TTL below the refetch interval, which production never
	// does: let every cache miss fetch.
	manager.refetchInterval = 0
	return manager
}

// A key removed from the JWKS endpoint stops being trusted once its cache TTL
// passes, however often it is used.
func TestManagerFetchKey_RemovedKeyExpires(t *testing.T) {
	const ttl = 200 * time.Millisecond
	_, pubKey, err := randomKeys(0)
	require.NoError(t, err)
	_, newPubKey, err := randomKeys(1)
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
	_, pubKey, err := randomKeys(0)
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
	_, pubKey1, err := randomKeys(0)
	require.NoError(t, err)
	_, pubKey2, err := randomKeys(1)
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
	_, pubKey, err := randomKeys(0)
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

// Lookups of kids missing in the cache fetch the endpoint at most once per
// refetch interval, however many different kids are asked for.
func TestManagerFetchKey_UnknownKidsFetchOncePerInterval(t *testing.T) {
	_, pubKey, err := randomKeys(0)
	require.NoError(t, err)
	ts := newRotatingJWKSServer(t, testKey{"kid", pubKey})
	manager, err := NewManager(ts.URL)
	require.NoError(t, err)
	manager.refetchInterval = time.Hour
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		_, err := manager.FetchKey(ctx, fmt.Sprintf("bogus-%d", i), nil)
		require.ErrorIs(t, err, ErrPublicKeyNotFound)
	}
	require.Equal(t, int32(1), ts.requests.Load())

	// Keys returned by that fetch are served from the cache.
	key, err := manager.FetchKey(ctx, "kid", nil)
	require.NoError(t, err)
	require.Equal(t, pubKey.N, parseRSA(t, key).N)
	require.Equal(t, int32(1), ts.requests.Load())
}

// Concurrent lookups of different unknown kids share one fetch.
func TestManagerFetchKey_ConcurrentUnknownKidsShareFetch(t *testing.T) {
	_, pubKey, err := randomKeys(0)
	require.NoError(t, err)
	release := make(chan struct{})
	var requests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-release
		jwksHandler(testKey{"kid", pubKey}).ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	manager, err := NewManager(ts.URL, WithHTTPClient(&http.Client{Timeout: 10 * time.Second}))
	require.NoError(t, err)
	ctx := context.Background()

	const n = 20
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			_, err := manager.FetchKey(ctx, fmt.Sprintf("bogus-%d", i), nil)
			errs <- err
		}(i)
	}
	require.Eventually(t, func() bool { return requests.Load() == 1 }, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // Let the other lookups join the fetch.
	close(release)
	for i := 0; i < n; i++ {
		require.ErrorIs(t, <-errs, ErrPublicKeyNotFound)
	}
	require.Equal(t, int32(1), requests.Load())
}

// A key the endpoint starts to return is picked up at most one refetch
// interval after the last fetch, however many unknown kids are asked for in
// between: rejected lookups do not extend the interval.
func TestManagerFetchKey_NewKeyPickedUpDespiteUnknownKids(t *testing.T) {
	const interval = 200 * time.Millisecond
	_, oldKey, err := randomKeys(0)
	require.NoError(t, err)
	_, newKey, err := randomKeys(1)
	require.NoError(t, err)
	ts := newRotatingJWKSServer(t, testKey{"old", oldKey})
	manager, err := NewManager(ts.URL)
	require.NoError(t, err)
	manager.refetchInterval = interval
	ctx := context.Background()

	_, err = manager.FetchKey(ctx, "old", nil)
	require.NoError(t, err)
	ts.setKeys(testKey{"old", oldKey}, testKey{"new", newKey})
	rotatedAt := time.Now()

	// Unknown kids keep arriving; the new key is found within the interval.
	var found bool
	for !found && time.Since(rotatedAt) < 5*interval {
		_, _ = manager.FetchKey(ctx, "bogus", nil)
		key, err := manager.FetchKey(ctx, "new", nil)
		if err == nil {
			require.Equal(t, newKey.N, parseRSA(t, key).N)
			found = true
		}
		time.Sleep(interval / 20)
	}
	require.True(t, found)
	// One interval at most, with slack for slow test machines.
	require.Less(t, time.Since(rotatedAt), 3*interval)
	// Fetches are bounded by the interval, not by the number of lookups.
	require.LessOrEqual(t, ts.requests.Load(), int32(3))
}

// A failed fetch does not start the interval: the endpoint is tried again
// once it is back.
func TestManagerFetchKey_FailedFetchDoesNotStartInterval(t *testing.T) {
	_, pubKey, err := randomKeys(0)
	require.NoError(t, err)
	ts := newRotatingJWKSServer(t, testKey{"kid", pubKey})
	ts.setDown()
	manager, err := NewManager(ts.URL)
	require.NoError(t, err)
	manager.refetchInterval = time.Hour
	ctx := context.Background()

	_, err = manager.FetchKey(ctx, "kid", nil)
	require.ErrorIs(t, err, errUnexpectedStatusCode)
	ts.setKeys(testKey{"kid", pubKey})
	_, err = manager.FetchKey(ctx, "kid", nil)
	require.NoError(t, err)
}

// The fetch time bookkeeping stays bounded whatever URLs token claims resolve
// the endpoint template to.
func TestManagerMarkFetchedBounded(t *testing.T) {
	manager, err := NewManager("https://example.com/{{tenant}}/jwks")
	require.NoError(t, err)
	manager.refetchInterval = time.Hour
	for i := 0; i < 3*_maxTrackedEndpoints; i++ {
		manager.markFetched(fmt.Sprintf("https://example.com/%d/jwks", i))
		require.LessOrEqual(t, len(manager.fetchedAt), _maxTrackedEndpoints)
	}
	require.True(t, manager.fetchedRecently(fmt.Sprintf("https://example.com/%d/jwks", 3*_maxTrackedEndpoints-1)))
}

// slowFirstMissCache makes the first cache miss return late, after another
// lookup could fetch the endpoint.
type slowFirstMissCache struct {
	Cache
	misses atomic.Int32
}

func (c *slowFirstMissCache) Get(cacheKey string) (*JWK, error) {
	key, err := c.Cache.Get(cacheKey)
	if err != nil && c.misses.Add(1) == 1 {
		time.Sleep(300 * time.Millisecond)
	}
	return key, err
}

// A lookup which missed the cache just before a concurrent fetch of the
// endpoint completed finds the fetched key instead of refusing it.
func TestManagerFetchKey_MissRacingConcurrentFetch(t *testing.T) {
	_, pubKey, err := randomKeys(0)
	require.NoError(t, err)
	ts := newRotatingJWKSServer(t, testKey{"kid", pubKey})
	ttlCache := NewTTLCache(time.Hour)
	t.Cleanup(func() { _ = ttlCache.Stop() })
	manager, err := NewManager(ts.URL, WithCache(&slowFirstMissCache{Cache: ttlCache}))
	require.NoError(t, err)
	manager.refetchInterval = time.Hour
	ctx := context.Background()

	slow := make(chan error, 1)
	go func() {
		_, err := manager.FetchKey(ctx, "kid", nil) // Misses, returns late.
		slow <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_, err = manager.FetchKey(ctx, "kid", nil) // Misses fast, fetches.
	require.NoError(t, err)
	require.NoError(t, <-slow)
	require.Equal(t, int32(1), ts.requests.Load())
}
