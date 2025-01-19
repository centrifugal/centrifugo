package jwtverify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// Use https://jwt.io to look at token contents.
// noinspection ALL
const (
	jwtValid                   = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn19.m-TaS80RxkAiP9jH_s_h2NrKS_TDuPxJ8-z6gI7UewI"
	jwtExpired                 = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImV4cCI6MTU4ODM1MTcwNH0.LTc0p5YlrwJcxXPETrjhm9qyYUBKCR5fSROmfCE4TD8"
	jwtNotBefore               = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImV4cCI6MzE3NjgyMDU3MCwibmJmIjozMTc2ODIwNTYwfQ.gfsQeznFw6g44OEnCTSBW7AkmLy92GBfXL_Bdvzs7vc"
	jwtInvalidSignature        = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImV4cCI6MTU4ODQxOTY5MywibmJmIjoxNTg4NDE4NjkzfQ.05Xj9adbLukdhSJFyiVUEgbxCHTajXuotmalFgYviCo"
	jwtArrayAud                = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImF1ZCI6WyJmb28iLCJiYXIiXX0.iY4pCPEQwstfNmPkLr7r7DrLZDo42q3E9jMc-TefI6g"
	jwtStringAud               = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImF1ZCI6ImZvbyJ9.jym6CG5haHME3ZQbb9jlnV1E0hSwwEjZycBZSygRzO0"
	jwtValidCustomUserClaim    = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoidGVzdCJ9.Mdh4PGRnqKD-8_cKCJOYKfi9KNLJz2PCKl3qEi0n0-w"
	subJWTValidCustomUserClaim = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoidGVzdCIsImNoYW5uZWwiOiJjaGFubmVsIn0.vMA6Ee2eq3d8ApAhbXmVv5LmArbrjFZgU2FUbK93EnQ"
	emptyObjectClaimsJWT       = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.e30.DMCAvRgzrcf5w0Z879BsqzcrnDFKBY_GN6c3qKOUFtQ"

	//
	// Generated with: https://github.com/lestrrat-go/jwx/tree/main/cmd/jwx
	//
	// ```
	// jwx jwk generate --type RSA --keysize 2048 --template '{"kid":"testrsa"}' > rsa.jwk
	// jwx jwk generate --type EC --curve P-384 --template '{"kid":"testec"}' > ec.jwk
	// jwx jwk generate --type OKP --curve Ed25519 --template '{"kid":"tested"}' > okp.jwk
	// jwx jwk generate --type EC --curve P-384 --template '{"kid":"fakeid"}' > fakeid.jwk
	// printf '{"sub":"2694","info":{"first_name":"Alexander","last_name":"Emelin"}}' > payload.txt
	// ```

	// JWK Set with RSA, EC and OKP keys:
	// ```
	// jwx jwk fmt --public-key rsa.jwk
	// jwx jwk fmt --public-key ec.jwk
	// jwx jwk fmt --public-key okp.jwk
	// ```
	jwksSet = `{
		  "keys": [
			  {
				"e": "AQAB",
				"kid": "testrsa",
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"n": "4Wp1fHDDOGN7rH357ofNfK26LDOA36ZtQ0H2x0uo12VsAxbGOfl67gES28ClWon9dSwGLR-urfAmX7DcCgffLMTgwCwvsPYCKsVIWMPvlGEPyAG90d55GVqJGpAYirfIVyjKkzJKIjqdmPx12XnjnrhWdTLl09Ja4E6SF5m1Ff4mkfavigrnuh_SaB1QKkMKj--ie0rH3VV9MAiQTnYkVuNPEEkz9h2SCyOMUmYLJMLIHIpWBZ4fI-XlCmFx_kGUgiU85m9lSoKFSl7zmvYvy-uxCteO_28COu-wLnhcN4uumnQKN13ESPXLtR7_fkP-Z-xlXoKMdZQfuWY6zc6AsQ"
			  },
			  {
				"crv": "P-384",
				"kid": "testec",
				"kty": "EC",
				"use": "sig",
				"alg": "ES384",
				"x": "W0A0VvKCnxs0trdgchvdkrEVfdjDYOeTdu_f0l3GE94LXBvVF_2O1Ng7vKZPE3cu",
				"y": "thDvOCDapgLR4krw5KKzp9HrkzTVgVwmwP37aTSc20EXw3R2fZ7tSh1ws3V7NV5n"
			  },
			  {
				"crv": "Ed25519",
				"d": "hgIMcVff-mdWy5xYFBqrkleEGVSiQu81GQwNxGxhj9k",
				"kid": "tested",
				"kty": "OKP",
				"use": "sig",
				"alg" : "EdDSA",
				"x": "KBbdGhSAMLXMh6zLMfGi4_4-npVhnEVnGVYdjSrOreI"
			  }
			]
		}`

	// JWT using `testrsa` key ID
	// ```
	// jwx jws sign --key rsa.jwk --alg RS256 payload.txt
	// ```
	jwtRSAWithKID = "eyJhbGciOiJSUzI1NiIsImtpZCI6InRlc3Ryc2EifQ.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn19.Kq6upfpD3WdTpupDH0PL9lgjAr5OUO0RMB_w6F6M67qWH36noeXXZ1G-_BlOow-a9Lwrdtiy5-WVXC3-wbADaNsOoMSLWWGH-9lxqQ5Df9cJoUu4Ocvm9S0vSB9G2e0yRqwKXL-UC_M1BZonHIwBPs5lCmU_1LynkGJ0uZSvVI8Ke5VQwLFeBsxjNpCbkVc0mAlES4uoD49nh0rTqXmyNoPcriFtxEyUIvdVHe8bKkEBC4yEhg7IevApD8BjY4M6btcOlZYmXxdVSHnNsShQS1WhVhTyqxYDj4ad7rNOZYz4_PE0hEKqSyNxNuJpYimc7aBImCDTgcewJh43Vygcdw"

	// JWT using `testec` key ID
	// ```
	// jwx jws sign --key ec.jwk --alg ES384 payload.txt
	// ```
	jwtECWithKID = "eyJhbGciOiJFUzM4NCIsImtpZCI6InRlc3RlYyJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn19.MFqUDsu8JjS99oROB_93JEwuGlWOnrmMK455UIjWDlSS62uwED7hE04ZcWyQClxnH88Z-oiU_qGnpTyi7zojMN9r3zBxXqdq-wyTqUH6P9NA5PIlXhh6pugLpGKwyfQG"

	// JWT using `tested` key ID
	// ```
	// jwx jws sign --key okp.jwk --alg EdDSA payload.txt
	// ```
	jwtOKPWithKID = "eyJhbGciOiJFZERTQSIsImtpZCI6InRlc3RlZCJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn19.upmjjFmTOk9A-gtkA8fPqfj0YAsETvXYotLNFrf86AyHBpQNRMw2ScKbRx6BHnG0klCbJ0C2dfJt501zx5WhCA"

	// JWT using `fakekid` key ID
	// ```
	// jwx jws sign --key ec.jwk --alg ES384 payload.txt
	// ```
	jwtECWithKIDMiss = "eyJhbGciOiJFUzM4NCIsImtpZCI6ImZha2VpZCJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn19.bmsj2WNZK1wnbzXs8pKRjvljHIgQK9g4N7_M2-tD5PAXpHqiP7ttqsyflIOOAbUVdIix4JOL_Dtv4GQsEof87AOsi--fPFaXkVXFQzx5MggshXn02FbwHQ0Mj6LOw1qC"

	// JWT using `testec` key ID but wrong alg
	// ```
	// jwx jws sign --key ec.jwk --alg ES256 payload.txt
	// ```
	jwtECWithKIDWrongAlg = "eyJhbGciOiJFUzI1NiIsImtpZCI6InRlc3RlYyJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn19.P7vuso8O-x7yzobSlfbyA7JiscnViLl303-5kAO7_6CZ3jWAOaZqllOrZfCTJgCoZlX5lNGgW6naJ3zKuOkO434osi6VeN1oaUl0-IHysmlP7Za5QNXzfUoKN6aCuJx7"
)

const nullByte = 0x0

func encodeToString(src []byte) string {
	return base64.RawURLEncoding.EncodeToString(src)
}

func encodeUint64ToString(v uint64) string {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, v)

	i := 0
	for ; i < len(data); i++ {
		if data[i] != nullByte {
			break
		}
	}

	return encodeToString(data[i:])
}

func generateTestRSAKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	reader := rand.Reader
	bitSize := 2048
	key, err := rsa.GenerateKey(reader, bitSize)
	require.NoError(t, err)
	return key, &key.PublicKey
}

func generateTestECDSAKeys(t *testing.T) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	reader := rand.Reader
	key, err := ecdsa.GenerateKey(elliptic.P256(), reader)
	require.NoError(t, err)
	return key, &key.PublicKey
}

func getRSATokenBuilder(rsaPrivateKey *rsa.PrivateKey, opts ...jwt.BuilderOption) *jwt.Builder {
	var signer jwt.Signer
	if rsaPrivateKey != nil {
		signer, _ = jwt.NewSignerRS(jwt.RS256, rsaPrivateKey)
	} else {
		// For HS we do everything in tests with key `secret`.
		key := []byte(`secret`)
		signer, _ = jwt.NewSignerHS(jwt.HS256, key)
	}
	return jwt.NewBuilder(signer, opts...)
}

func getECDSATokenBuilder(ecdsaPrivateKey *ecdsa.PrivateKey, opts ...jwt.BuilderOption) *jwt.Builder {
	var signer jwt.Signer
	if ecdsaPrivateKey != nil {
		signer, _ = jwt.NewSignerES(jwt.ES256, ecdsaPrivateKey)
	} else {
		// For HS we do everything in tests with key `secret`.
		key := []byte(`secret`)
		signer, _ = jwt.NewSignerHS(jwt.HS256, key)

	}
	return jwt.NewBuilder(signer, opts...)
}

func getRSAConnToken(user string, exp int64, rsaPrivateKey *rsa.PrivateKey, opts ...jwt.BuilderOption) string {
	builder := getRSATokenBuilder(rsaPrivateKey, opts...)
	claims := &ConnectTokenClaims{
		Base64Info: "e30=",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  user,
			Audience: []string{"test"},
			Issuer:   "test",
		},
	}
	if exp > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Unix(exp, 0))
	}
	token, err := builder.Build(claims)
	if err != nil {
		panic(err)
	}
	return token.String()
}

func getECDSAConnToken(user string, exp int64, ecdsaPrivateKey *ecdsa.PrivateKey, opts ...jwt.BuilderOption) string {
	builder := getECDSATokenBuilder(ecdsaPrivateKey, opts...)
	claims := &ConnectTokenClaims{
		Base64Info: "e30=",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: user,
		},
	}
	if exp > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Unix(exp, 0))
	}
	token, err := builder.Build(claims)
	if err != nil {
		panic(err)
	}
	return token.String()
}

func getRSASubscribeToken(channel string, client string, exp int64, rsaPrivateKey *rsa.PrivateKey) string {
	builder := getRSATokenBuilder(rsaPrivateKey)
	claims := &SubscribeTokenClaims{
		SubscribeOptions: SubscribeOptions{
			Base64Info: "e30=",
		},
		Channel:          channel,
		Client:           client,
		RegisteredClaims: jwt.RegisteredClaims{},
	}
	if exp > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Unix(exp, 0))
	}
	token, err := builder.Build(claims)
	if err != nil {
		panic(err)
	}
	return token.String()
}

func getECDSASubscribeToken(channel string, client string, exp int64, ecdsaPrivateKey *ecdsa.PrivateKey) string {
	builder := getECDSATokenBuilder(ecdsaPrivateKey)
	claims := &SubscribeTokenClaims{
		SubscribeOptions: SubscribeOptions{
			Base64Info: "e30=",
		},
		Channel:          channel,
		Client:           client,
		RegisteredClaims: jwt.RegisteredClaims{},
	}
	if exp > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Unix(exp, 0))
	}
	token, err := builder.Build(claims)
	if err != nil {
		panic(err)
	}
	return token.String()
}

func getJWKServer(pubKey *rsa.PublicKey, kty, use, kid string) *httptest.Server {
	return httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"keys": []map[string]string{
				{
					"alg": "RS256",
					"kty": kty,
					"use": use,
					"kid": kid,
					"n":   encodeToString(pubKey.N.Bytes()),
					"e":   encodeUint64ToString(uint64(pubKey.E)),
				},
			}}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func Test_tokenVerifierJWT_Signer(t *testing.T) {
	_, rsaPubKey := generateTestRSAKeys(t)
	_, ecdsaPubKey := generateTestECDSAKeys(t)
	signer, err := newAlgorithms("secret", rsaPubKey, ecdsaPubKey)
	require.NoError(t, err)
	require.NotNil(t, signer)
}

func Test_tokenVerifierJWT_Valid(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtValid, false)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
	require.NotNil(t, ct.Info)
	require.Equal(t, `{"first_name":"Alexander","last_name":"Emelin"}`, string(ct.Info))
}

func Test_tokenVerifierJWT_Valid_CustomClaim(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	// Test that by default `user_id` claim is ignored.
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtValidCustomUserClaim, false)
	require.NoError(t, err)
	require.Equal(t, "", ct.UserID)

	// Now test that custom `user_id` claim works for connection token.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", "user_id"}, cfgContainer)
	require.NoError(t, err)
	ct, err = verifier.VerifyConnectToken(jwtValidCustomUserClaim, false)
	require.NoError(t, err)
	require.Equal(t, "test", ct.UserID)

	// And the same for subscription token.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	st, err := verifier.VerifySubscribeToken(subJWTValidCustomUserClaim, false)
	require.NoError(t, err)
	require.Equal(t, "", st.UserID)
	require.Equal(t, "channel", st.Channel)

	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", "user_id"}, cfgContainer)
	require.NoError(t, err)
	st, err = verifier.VerifySubscribeToken(subJWTValidCustomUserClaim, false)
	require.NoError(t, err)
	require.Equal(t, "test", st.UserID)

	// Also make sure custom claim returns empty user ID from empty object claims token.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", "user_id"}, cfgContainer)
	require.NoError(t, err)
	ct, err = verifier.VerifyConnectToken(emptyObjectClaimsJWT, false)
	require.NoError(t, err)
	require.Equal(t, "", ct.UserID)
}

func Test_tokenVerifierJWT_Audience(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "test2", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)

	// Token without aud.
	_, err = verifier.VerifyConnectToken(jwtValid, false)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Generate token with aud.
	token := getRSAConnToken("user", time.Now().Add(time.Hour).Unix(), nil)

	// Verifier with audience which does not match aud in token.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "test2", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)

	_, err = verifier.VerifyConnectToken(token, false)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Verifier with token audience.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "test", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token, false)
	require.NoError(t, err)

	// Verifier with token audience - valid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "test", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token, false)
	require.NoError(t, err)

	// Verifier with token audience - invalid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "test2", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token, false)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_Issuer(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "test2", "", ""}, cfgContainer)
	require.NoError(t, err)

	// Token without iss.
	_, err = verifier.VerifyConnectToken(jwtValid, false)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Generate token with iss.
	token := getRSAConnToken("user", time.Now().Add(time.Hour).Unix(), nil)

	// Verifier with issuer which does not match token iss.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "test2", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token, false)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Verifier with token issuer.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "test", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token, false)
	require.NoError(t, err)

	// Verifier with token issuer regex - valid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "test", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token, false)
	require.NoError(t, err)

	// Verifier with token issuer regex - invalid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "test2", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token, false)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_Expired(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtExpired, false)
	require.Error(t, err)
	require.Equal(t, ErrTokenExpired, err)
}

func Test_tokenVerifierJWT_DisabledAlgorithm(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtExpired, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidToken), err.Error())
}

func Test_tokenVerifierJWT_InvalidSignature(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtInvalidSignature, false)
	require.Error(t, err)

	// Test that skipVerify results into accepted token.
	ct, err := verifier.VerifyConnectToken(jwtValid+"xxx", true)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_InvalidSignature_SkipVerify(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtValid+"xxx", true)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_WithNotBefore(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtNotBefore, false)
	require.Error(t, err)

	// Test that skipVerify still results into unaccepted token if it's expired.
	_, err = verifier.VerifyConnectToken(jwtNotBefore, true)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_StringAudience(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtStringAud, false)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_ArrayAudience(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtArrayAud, false)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_VerifyConnectToken(t *testing.T) {
	type args struct {
		token string
	}

	rsaPrivateKey, rsaPubKey := generateTestRSAKeys(t)
	ecdsaPrivateKey, ecdsaPubKey := generateTestECDSAKeys(t)

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	verifierJWT, err := NewTokenVerifierJWT(VerifierConfig{"secret", rsaPubKey, ecdsaPubKey, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)

	_time := time.Now()
	tests := []struct {
		name     string
		verifier Verifier
		args     args
		want     ConnectToken
		wantErr  bool
		expired  bool
	}{
		{
			name:     "Valid JWT HS",
			verifier: verifierJWT,
			args: args{
				token: getRSAConnToken("user1", _time.Add(24*time.Hour).Unix(), nil),
			},
			want: ConnectToken{
				UserID:   "user1",
				ExpireAt: _time.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
				Subs:     map[string]centrifuge.SubscribeOptions{},
			},
			wantErr: false,
		}, {
			name:     "Valid JWT RS",
			verifier: verifierJWT,
			args: args{
				token: getRSAConnToken("user1", _time.Add(24*time.Hour).Unix(), rsaPrivateKey),
			},
			want: ConnectToken{
				UserID:   "user1",
				ExpireAt: _time.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
				Subs:     map[string]centrifuge.SubscribeOptions{},
			},
			wantErr: false,
		},
		{
			name:     "Valid JWT ES",
			verifier: verifierJWT,
			args: args{
				token: getECDSAConnToken("user1", _time.Add(24*time.Hour).Unix(), ecdsaPrivateKey),
			},
			want: ConnectToken{
				UserID:   "user1",
				ExpireAt: _time.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
				Subs:     map[string]centrifuge.SubscribeOptions{},
			},
			wantErr: false,
		},
		{
			name:     "Invalid JWT",
			verifier: verifierJWT,
			args: args{
				token: "Invalid jwt",
			},
			want:    ConnectToken{},
			wantErr: true,
			expired: false,
		}, {
			name:     "Expired JWT",
			verifier: verifierJWT,
			args: args{
				token: getRSAConnToken("user1", _time.Add(-24*time.Hour).Unix(), nil),
			},
			want:    ConnectToken{},
			wantErr: true,
			expired: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.verifier.VerifyConnectToken(tt.args.token, false)
			if tt.wantErr && err == nil {
				t.Errorf("VerifyConnectToken() should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("VerifyConnectToken() should not return error")
			}
			if tt.expired && !errors.Is(err, ErrTokenExpired) {
				t.Errorf("VerifyConnectToken() should return token expired error")
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_tokenVerifierJWT_VerifyConnectTokenWithJWK(t *testing.T) {
	type token struct {
		user string
		exp  int64
	}

	type jwk struct {
		kty string
		use string
		kid string
	}

	now := time.Now()

	testCases := []struct {
		name    string
		token   token
		jwk     jwk
		want    ConnectToken
		wantErr bool
		expired bool
	}{
		{
			name:  "OK",
			token: token{user: "user1", exp: now.Add(24 * time.Hour).Unix()},
			jwk:   jwk{kty: "RSA", use: "sig", kid: "ok"},
			want: ConnectToken{
				UserID:   "user1",
				ExpireAt: now.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
				Subs:     map[string]centrifuge.SubscribeOptions{},
			},
			wantErr: false,
		},
		{
			name:    "ExpiredToken",
			token:   token{user: "user2", exp: now.Add(-24 * time.Hour).Unix()},
			jwk:     jwk{kty: "RSA", use: "sig", kid: "ok"},
			want:    ConnectToken{},
			wantErr: false,
			expired: true,
		},
		{
			name:    "InvalidKeyUsage",
			token:   token{user: "user3", exp: now.Add(24 * time.Hour).Unix()},
			jwk:     jwk{kty: "RSA", use: "enc", kid: "invalidkeyusage"},
			want:    ConnectToken{},
			wantErr: true,
		},
		{
			name:    "InvalidKeyType",
			token:   token{user: "user4", exp: now.Add(24 * time.Hour).Unix()},
			jwk:     jwk{kty: "HS", use: "sig", kid: "invalidkeytype"},
			want:    ConnectToken{},
			wantErr: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)

			privKey, pubKey := generateTestRSAKeys(t)
			ts := getJWKServer(pubKey, tt.jwk.kty, tt.jwk.use, tt.jwk.kid)

			ts.Start()
			defer ts.Close()

			cfg := config.DefaultConfig()
			cfgContainer, err := config.NewContainer(cfg)
			require.NoError(t, err)

			verifier, err := NewTokenVerifierJWT(VerifierConfig{"", nil, nil, ts.URL, "", "", "", "", ""}, cfgContainer)
			require.NoError(t, err)

			token := getRSAConnToken(tt.token.user, tt.token.exp, privKey, jwt.WithKeyID(tt.jwk.kid))

			got, err := verifier.VerifyConnectToken(token, false)
			if tt.wantErr {
				r.Error(err)
				return
			}

			if tt.expired {
				r.EqualError(err, ErrTokenExpired.Error())
				return
			}

			r.NoError(err)
			r.EqualValues(got, tt.want)
		})
	}
}

func Test_tokenVerifierJWT_VerifySubscribeToken(t *testing.T) {
	type args struct {
		token string
	}

	rsaPrivateKey, rsaPubKey := generateTestRSAKeys(t)
	ecdsaPrivateKey, ecdsaPubKey := generateTestECDSAKeys(t)

	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)

	verifierJWT, err := NewTokenVerifierJWT(VerifierConfig{"secret", rsaPubKey, ecdsaPubKey, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)

	_time := time.Now()
	tests := []struct {
		name       string
		verifier   Verifier
		args       args
		want       SubscribeToken
		wantErr    bool
		expired    bool
		skipVerify bool
	}{
		{
			name:     "Empty JWT",
			verifier: verifierJWT,
			args:     args{},
			want:     SubscribeToken{},
			wantErr:  true,
			expired:  false,
		}, {
			name:     "Invalid JWT",
			verifier: verifierJWT,
			args: args{
				token: "randomToken",
			},
			want:    SubscribeToken{},
			wantErr: true,
			expired: false,
		}, {
			name:     "Expired JWT",
			verifier: verifierJWT,
			args: args{
				token: getRSASubscribeToken("channel1", "client1", _time.Add(-24*time.Hour).Unix(), nil),
			},
			want:    SubscribeToken{},
			wantErr: true,
			expired: true,
		}, {
			name:     "Valid JWT HS",
			verifier: verifierJWT,
			args: args{
				token: getRSASubscribeToken("channel1", "client1", _time.Add(24*time.Hour).Unix(), nil),
			},
			want: SubscribeToken{
				Client:  "client1",
				Channel: "channel1",
				Options: centrifuge.SubscribeOptions{
					ExpireAt:    _time.Add(24 * time.Hour).Unix(),
					ChannelInfo: []byte("{}"),
				},
			},
			wantErr: false,
		}, {
			name:     "Invalid JWT HS",
			verifier: verifierJWT,
			args: args{
				token: getRSASubscribeToken("channel1", "client1", _time.Add(24*time.Hour).Unix(), nil) + "xxx",
			},
			want:       SubscribeToken{},
			wantErr:    true,
			skipVerify: false,
		}, {
			name:     "Invalid JWT HS but verify skipped",
			verifier: verifierJWT,
			args: args{
				token: getRSASubscribeToken("channel1", "client1", _time.Add(24*time.Hour).Unix(), nil) + "xxx",
			},
			want: SubscribeToken{
				Client:  "client1",
				Channel: "channel1",
				Options: centrifuge.SubscribeOptions{
					ExpireAt:    _time.Add(24 * time.Hour).Unix(),
					ChannelInfo: []byte("{}"),
				},
			},
			wantErr:    false,
			skipVerify: true,
		}, {
			name:     "Valid JWT RS",
			verifier: verifierJWT,
			args: args{
				token: getRSASubscribeToken("channel1", "client1", _time.Add(24*time.Hour).Unix(), rsaPrivateKey),
			},
			want: SubscribeToken{
				Client:  "client1",
				Channel: "channel1",
				Options: centrifuge.SubscribeOptions{
					ExpireAt:    _time.Add(24 * time.Hour).Unix(),
					ChannelInfo: []byte("{}"),
				},
			},
			wantErr: false,
		},
		{
			name:     "Valid JWT ES",
			verifier: verifierJWT,
			args: args{
				token: getECDSASubscribeToken("channel1", "client1", _time.Add(24*time.Hour).Unix(), ecdsaPrivateKey),
			},
			want: SubscribeToken{
				Client:  "client1",
				Channel: "channel1",
				Options: centrifuge.SubscribeOptions{
					ExpireAt:    _time.Add(24 * time.Hour).Unix(),
					ChannelInfo: []byte("{}"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.verifier.VerifySubscribeToken(tt.args.token, tt.skipVerify)
			if tt.wantErr && err == nil {
				t.Errorf("VerifySubscribeToken() should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("VerifySubscribeToken() should not return error, but returned: %v", err)
			}
			if tt.expired && !errors.Is(err, ErrTokenExpired) {
				t.Errorf("VerifySubscribeToken() should return token expired error")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VerifySubscribeToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func jwksHandler(json string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(json))
	})
}

func TestJWKS(t *testing.T) {
	// Create a test JWKS server.
	ts := httptest.NewServer(jwksHandler(jwksSet))
	defer ts.Close()

	// Setup our token verifier, using the test JWKS endpoint
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"", nil, nil, ts.URL, "", "", "", "", ""}, cfgContainer)
	require.NoError(t, err)

	// Validate an RSA token
	ct, err := verifier.VerifyConnectToken(jwtRSAWithKID, false)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
	require.NotNil(t, ct.Info)
	require.Equal(t, `{"first_name":"Alexander","last_name":"Emelin"}`, string(ct.Info))

	// Validate an EC token
	ct, err = verifier.VerifyConnectToken(jwtECWithKID, false)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
	require.NotNil(t, ct.Info)
	require.Equal(t, `{"first_name":"Alexander","last_name":"Emelin"}`, string(ct.Info))

	// Validate OKP (based on EdDSA) token.
	ct, err = verifier.VerifyConnectToken(jwtOKPWithKID, false)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
	require.NotNil(t, ct.Info)
	require.Equal(t, `{"first_name":"Alexander","last_name":"Emelin"}`, string(ct.Info))

	// Make sure a token with an unknown KID fails to verify
	_, err = verifier.VerifyConnectToken(jwtECWithKIDMiss, false)
	require.ErrorContains(t, err, "invalid token: jwks: public key not found")

	// Make sure a token with an unknown KID fails to verify
	_, err = verifier.VerifyConnectToken(jwtECWithKIDWrongAlg, false)
	require.ErrorContains(t, err, "invalid token: token is signed by another algorithm")
}

func BenchmarkConnectTokenVerify_Valid(b *testing.B) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(b, err)
	verifierJWT, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := verifierJWT.VerifyConnectToken(jwtValid, false)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportAllocs()
}

func BenchmarkConnectTokenVerify_Expired(b *testing.B) {
	cfg := config.DefaultConfig()
	cfgContainer, err := config.NewContainer(cfg)
	require.NoError(b, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "", ""}, cfgContainer)
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		_, err := verifier.VerifyConnectToken(jwtExpired, false)
		if err != ErrTokenExpired {
			panic(err)
		}
	}
}
