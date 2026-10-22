package jwtverify

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

func getEdDSAToken(t *testing.T, key ed25519.PrivateKey, claims any) string {
	t.Helper()
	signer, err := jwt.NewSignerEdDSA(key)
	require.NoError(t, err)
	token, err := jwt.NewBuilder(signer).Build(claims)
	require.NoError(t, err)
	return token.String()
}

func newEdDSAVerifier(t *testing.T, cfg VerifierConfig) *VerifierJWT {
	t.Helper()
	cfgContainer, err := config.NewContainer(config.DefaultConfig())
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(cfg, cfgContainer)
	require.NoError(t, err)
	return verifier
}

// Connection and subscription tokens signed with EdDSA are verified with the
// configured Ed25519 public key, next to the other keys.
func Test_tokenVerifierJWT_EdDSA(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, ecdsaPubKey := generateTestECDSAKeys(t)
	verifier := newEdDSAVerifier(t, VerifierConfig{HMACSecretKey: "secret", ECDSAPublicKey: ecdsaPubKey, Ed25519PublicKey: pub})

	exp := jwt.NewNumericDate(time.Now().Add(time.Hour))
	ct, err := verifier.VerifyConnectToken(getEdDSAToken(t, priv, &ConnectTokenClaims{
		Info:             []byte(`{"name":"Alice"}`),
		RegisteredClaims: jwt.RegisteredClaims{Subject: "42", ExpiresAt: exp},
	}), false)
	require.NoError(t, err)
	require.Equal(t, "42", ct.UserID)
	require.Equal(t, `{"name":"Alice"}`, string(ct.Info))
	require.Equal(t, exp.Unix(), ct.ExpireAt)

	st, err := verifier.VerifySubscribeToken(getEdDSAToken(t, priv, &SubscribeTokenClaims{
		Channel:          "channel1",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "42", ExpiresAt: exp},
	}), false)
	require.NoError(t, err)
	require.Equal(t, "42", st.UserID)
	require.Equal(t, "channel1", st.Channel)

	// HMAC tokens still verify alongside.
	_, err = verifier.VerifyConnectToken(getECDSAConnToken("user1", time.Now().Add(time.Hour).Unix(), nil), false)
	require.NoError(t, err)
}

// An EdDSA token is refused when no Ed25519 key is configured, and when
// signed with another key.
func Test_tokenVerifierJWT_EdDSA_Refused(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	claims := &ConnectTokenClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "42"}}

	_, err = newEdDSAVerifier(t, VerifierConfig{HMACSecretKey: "secret"}).VerifyConnectToken(getEdDSAToken(t, priv, claims), false)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidToken), err.Error())

	_, err = newEdDSAVerifier(t, VerifierConfig{Ed25519PublicKey: pub}).VerifyConnectToken(getEdDSAToken(t, otherPriv, claims), false)
	require.Error(t, err)
}
