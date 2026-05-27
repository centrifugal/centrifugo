package jwtverify

import (
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// TestJWKSCacheKeyIsScopedToTemplatedEndpoint is a regression test for a
// cross-issuer JWT verification bypass. With templated JWKS endpoints
// (e.g., per-tenant URLs derived from the JWT iss claim), the JWKS cache
// must scope entries by the resolved endpoint, not only by the JWT header
// kid. Otherwise, a key cached from tenant A's endpoint could be used to
// verify a token forged for tenant B if both JWKS documents advertise the
// same kid — kid is not globally unique by spec.
//
// Scenario: an attacker controls tenant A and mints a token claiming
// issuer "tenant-b" but signed with tenant A's key. Tenant A's key is
// already cached. The verifier must NOT accept the forged token.
func TestJWKSCacheKeyIsScopedToTemplatedEndpoint(t *testing.T) {
	const kid = "shared-kid"

	tenantAPrivateKey, tenantAPublicKey := generateTestRSAKeys(t)
	_, tenantBPublicKey := generateTestRSAKeys(t)

	var tenantARequests, tenantBRequests int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tenant-a/jwks.json":
			atomic.AddInt32(&tenantARequests, 1)
			writeRSAJWKS(t, w, tenantAPublicKey, kid)
		case "/tenant-b/jwks.json":
			atomic.AddInt32(&tenantBRequests, 1)
			writeRSAJWKS(t, w, tenantBPublicKey, kid)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	verifier, err := NewTokenVerifierJWT(VerifierConfig{
		JWKSPublicEndpoint: ts.URL + "/{{tenant}}/jwks.json",
		IssuerRegex:        `^(?P<tenant>tenant-a|tenant-b)$`,
	}, cfgContainer)
	require.NoError(t, err)

	legitimateTenantAToken := getRSAIssuerConnToken(t, "tenant-a-user", "tenant-a", tenantAPrivateKey, kid)
	forgedTenantBToken := getRSAIssuerConnToken(t, "victim", "tenant-b", tenantAPrivateKey, kid)

	// Step 1: Legitimate tenant-A token primes tenant A's key in the cache.
	ct, err := verifier.VerifyConnectToken(legitimateTenantAToken, false)
	require.NoError(t, err)
	require.Equal(t, "tenant-a-user", ct.UserID)
	require.Equal(t, int32(1), atomic.LoadInt32(&tenantARequests))
	require.Equal(t, int32(0), atomic.LoadInt32(&tenantBRequests))

	// Step 2: Forged tenant-B token (signed by tenant A) must be rejected.
	// The verifier must consult tenant B's JWKS endpoint, fetch tenant B's
	// public key, and reject the signature.
	_, err = verifier.VerifyConnectToken(forgedTenantBToken, false)
	require.Error(t, err, "forged cross-tenant token must be rejected")
	require.ErrorIs(t, err, ErrInvalidToken)
	require.Equal(t, int32(1), atomic.LoadInt32(&tenantBRequests),
		"verifier must fetch tenant B's JWKS rather than reuse cached tenant A key")
}

func writeRSAJWKS(t *testing.T, w http.ResponseWriter, pubKey *rsa.PublicKey, kid string) {
	t.Helper()
	resp := map[string]any{
		"keys": []map[string]string{
			{
				"alg": "RS256",
				"kty": "RSA",
				"use": "sig",
				"kid": kid,
				"n":   encodeToString(pubKey.N.Bytes()),
				"e":   encodeUint64ToString(uint64(pubKey.E)),
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(resp))
}

func getRSAIssuerConnToken(t *testing.T, user, issuer string, rsaPrivateKey *rsa.PrivateKey, kid string) string {
	t.Helper()
	signer, err := jwt.NewSignerRS(jwt.RS256, rsaPrivateKey)
	require.NoError(t, err)
	builder := jwt.NewBuilder(signer, jwt.WithKeyID(kid))
	claims := &ConnectTokenClaims{
		Base64Info: "e30=",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user,
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := builder.Build(claims)
	require.NoError(t, err)
	return token.String()
}
