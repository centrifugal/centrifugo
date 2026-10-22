package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/jwtverify"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// A connection token checked against a JWKS endpoint, as a client sees the
// outcome: a key the endpoint does not have yet - a rotation in progress - and
// an endpoint which can't be used are both recoverable, so that clients retry;
// only a bad signature disconnects for good.
func TestClientConnectingJWKSErrors(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	var up atomic.Bool
	up.Store(true)
	enc := base64.RawURLEncoding
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintf(w, `{"keys":[{"kty":"RSA","kid":"known","use":"sig","alg":"RS256","n":%q,"e":%q}]}`,
			enc.EncodeToString(key.PublicKey.N.Bytes()), enc.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()))
	}))
	defer ts.Close()

	node := tools.NodeWithMemoryEngine()
	defer func() { _ = node.Shutdown(context.Background()) }()
	cfgContainer, err := config.NewContainer(config.DefaultConfig())
	require.NoError(t, err)
	verifier, err := jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{JWKSPublicEndpoint: ts.URL}, cfgContainer)
	require.NoError(t, err)
	h := NewHandler(node, cfgContainer, verifier, nil, &ProxyMap{})

	token := func(signWith *rsa.PrivateKey, kid string) string {
		signer, err := jwt.NewSignerRS(jwt.RS256, signWith)
		require.NoError(t, err)
		tok, err := jwt.NewBuilder(signer, jwt.WithKeyID(kid)).Build(&jwtverify.ConnectTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "42", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		})
		require.NoError(t, err)
		return tok.String()
	}
	connect := func(tok string) error {
		_, err := h.OnClientConnecting(context.Background(), centrifuge.ConnectEvent{
			Transport: tools.NewTestTransport(),
			Token:     tok,
		}, nil, false)
		return err
	}

	// A key the endpoint does not have: the client gets a new token.
	require.Equal(t, centrifuge.ErrorTokenExpired, connect(token(key, "unknown")))

	// Another key under a known kid: invalid, for good.
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	require.Equal(t, centrifuge.DisconnectInvalidToken, connect(token(other, "known")))

	// The endpoint failing before any key was fetched: a temporary error, not
	// an invalid token - the client reconnects.
	verifier2, err := jwtverify.NewTokenVerifierJWT(jwtverify.VerifierConfig{JWKSPublicEndpoint: ts.URL}, cfgContainer)
	require.NoError(t, err)
	h = NewHandler(node, cfgContainer, verifier2, nil, &ProxyMap{})
	up.Store(false)
	err = connect(token(key, "known"))
	require.Error(t, err)
	var disconnect centrifuge.Disconnect
	require.False(t, errors.As(err, &disconnect), "disconnect %v", err)
	require.NotEqual(t, centrifuge.ErrorTokenExpired, err)
}
