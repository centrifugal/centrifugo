package jwks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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
