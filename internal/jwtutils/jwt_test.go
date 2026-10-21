package jwtutils

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func pemPublicKey(t *testing.T, pub any) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestParseEd25519PublicKeyFromPEM(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	got, err := ParseEd25519PublicKeyFromPEM(pemPublicKey(t, pub))
	require.NoError(t, err)
	require.True(t, pub.Equal(got))

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	_, err = ParseEd25519PublicKeyFromPEM(pemPublicKey(t, &ecKey.PublicKey))
	require.True(t, errors.Is(err, errNotEd25519PublicKey), err)

	_, err = ParseEd25519PublicKeyFromPEM([]byte("not a key"))
	require.True(t, errors.Is(err, errKeyMustBePEMEncoded), err)
}
