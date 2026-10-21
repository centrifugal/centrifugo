package jwtverify

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/jwks"

	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// rsaJWK is the JWK of an RSA public key, as a JWKS endpoint serves it.
func rsaJWK(kid string, pub *rsa.PublicKey) string {
	enc := base64.RawURLEncoding
	return fmt.Sprintf(`{"kty":"RSA","kid":%q,"use":"sig","alg":"RS256","n":%q,"e":%q}`,
		kid, enc.EncodeToString(pub.N.Bytes()), enc.EncodeToString(big.NewInt(int64(pub.E)).Bytes()))
}

func rsaTokenWithKID(t *testing.T, key *rsa.PrivateKey, kid string, claims any) string {
	t.Helper()
	signer, err := jwt.NewSignerRS(jwt.RS256, key)
	require.NoError(t, err)
	token, err := jwt.NewBuilder(signer, jwt.WithKeyID(kid)).Build(claims)
	require.NoError(t, err)
	return token.String()
}

// jwksErrorsSetup serves the JWK of key under "known" while up is set, and
// fails with status 500 otherwise.
func jwksErrorsSetup(t *testing.T) (*VerifierJWT, *rsa.PrivateKey, *atomic.Bool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	var up atomic.Bool
	up.Store(true)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"keys":[` + rsaJWK("known", &key.PublicKey) + `]}`))
	}))
	t.Cleanup(ts.Close)
	cfgContainer, err := config.NewContainer(config.DefaultConfig())
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{JWKSPublicEndpoint: ts.URL}, cfgContainer)
	require.NoError(t, err)
	return verifier, key, &up
}

// A token whose key the JWKS endpoint does not have - yet, during a rotation,
// or any more - is expired, not invalid: clients answer with a new token, and
// so recover. A token signed by another key under a known kid stays invalid.
func Test_tokenVerifierJWT_JWKSKeyNotFound(t *testing.T) {
	verifier, key, _ := jwksErrorsSetup(t)
	exp := jwt.NewNumericDate(time.Now().Add(time.Hour))
	conn := &ConnectTokenClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "42", ExpiresAt: exp}}
	sub := &SubscribeTokenClaims{Channel: "ch", RegisteredClaims: jwt.RegisteredClaims{Subject: "42", ExpiresAt: exp}}

	_, err := verifier.VerifyConnectToken(rsaTokenWithKID(t, key, "unknown", conn), false)
	require.ErrorIs(t, err, ErrTokenExpired)
	require.NotErrorIs(t, err, ErrInvalidToken)
	_, err = verifier.VerifySubscribeToken(rsaTokenWithKID(t, key, "unknown", sub), false)
	require.ErrorIs(t, err, ErrTokenExpired)

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(rsaTokenWithKID(t, other, "known", conn), false)
	require.ErrorIs(t, err, ErrInvalidToken)
	_, err = verifier.VerifySubscribeToken(rsaTokenWithKID(t, other, "known", sub), false)
	require.ErrorIs(t, err, ErrInvalidToken)

	_, err = verifier.VerifyConnectToken(rsaTokenWithKID(t, key, "known", conn), false)
	require.NoError(t, err)
}

// A JWKS endpoint which can't be used fails verification temporarily: the
// error is neither an invalid nor an expired token, so clients retry later.
func Test_tokenVerifierJWT_JWKSUnavailable(t *testing.T) {
	verifier, key, up := jwksErrorsSetup(t)
	up.Store(false)
	exp := jwt.NewNumericDate(time.Now().Add(time.Hour))
	conn := &ConnectTokenClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "42", ExpiresAt: exp}}
	sub := &SubscribeTokenClaims{Channel: "ch", RegisteredClaims: jwt.RegisteredClaims{Subject: "42", ExpiresAt: exp}}

	for name, verify := range map[string]func() error{
		"connection": func() error {
			_, err := verifier.VerifyConnectToken(rsaTokenWithKID(t, key, "known", conn), false)
			return err
		},
		"subscription": func() error {
			_, err := verifier.VerifySubscribeToken(rsaTokenWithKID(t, key, "known", sub), false)
			return err
		},
	} {
		err := verify()
		require.ErrorIs(t, err, jwks.ErrUnavailable, name)
		require.NotErrorIs(t, err, ErrInvalidToken, name)
		require.NotErrorIs(t, err, ErrTokenExpired, name)
	}
}
