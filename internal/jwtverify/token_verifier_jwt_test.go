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

	"github.com/centrifugal/centrifugo/v5/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/cristalhq/jwt/v5"
	"github.com/stretchr/testify/require"
)

// Use https://jwt.io to look at token contents.
// noinspection ALL
const (
	jwtValid            = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn19.m-TaS80RxkAiP9jH_s_h2NrKS_TDuPxJ8-z6gI7UewI"
	jwtExpired          = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImV4cCI6MTU4ODM1MTcwNH0.LTc0p5YlrwJcxXPETrjhm9qyYUBKCR5fSROmfCE4TD8"
	jwtNotBefore        = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImV4cCI6MzE3NjgyMDU3MCwibmJmIjozMTc2ODIwNTYwfQ.gfsQeznFw6g44OEnCTSBW7AkmLy92GBfXL_Bdvzs7vc"
	jwtInvalidSignature = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImV4cCI6MTU4ODQxOTY5MywibmJmIjoxNTg4NDE4NjkzfQ.05Xj9adbLukdhSJFyiVUEgbxCHTajXuotmalFgYviCo"
	jwtArrayAud         = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImF1ZCI6WyJmb28iLCJiYXIiXX0.iY4pCPEQwstfNmPkLr7r7DrLZDo42q3E9jMc-TefI6g"
	jwtStringAud        = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIyNjk0IiwiaW5mbyI6eyJmaXJzdF9uYW1lIjoiQWxleGFuZGVyIiwibGFzdF9uYW1lIjoiRW1lbGluIn0sImF1ZCI6ImZvbyJ9.jym6CG5haHME3ZQbb9jlnV1E0hSwwEjZycBZSygRzO0"
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
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtValid)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
	require.NotNil(t, ct.Info)
	require.Equal(t, `{"first_name":"Alexander","last_name":"Emelin"}`, string(ct.Info))
}

func Test_tokenVerifierJWT_Audience(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "test2", "", "", ""}, ruleContainer)
	require.NoError(t, err)

	// Token without aud.
	_, err = verifier.VerifyConnectToken(jwtValid)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Generate token with aud.
	token := getRSAConnToken("user", time.Now().Add(time.Hour).Unix(), nil)

	// Verifier with audience which does not match aud in token.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "test2", "", "", ""}, ruleContainer)
	require.NoError(t, err)

	_, err = verifier.VerifyConnectToken(token)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Verifier with token audience.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "test", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token)
	require.NoError(t, err)

	// Verifier with token audience - valid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "test", "", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token)
	require.NoError(t, err)

	// Verifier with token audience - invalid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "test2", "", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_Issuer(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "test2", ""}, ruleContainer)
	require.NoError(t, err)

	// Token without iss.
	_, err = verifier.VerifyConnectToken(jwtValid)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Generate token with iss.
	token := getRSAConnToken("user", time.Now().Add(time.Hour).Unix(), nil)

	// Verifier with issuer which does not match token iss.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "test2", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token)
	require.ErrorIs(t, err, ErrInvalidToken)

	// Verifier with token issuer.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "test", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token)
	require.NoError(t, err)

	// Verifier with token issuer regex - valid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "test"}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token)
	require.NoError(t, err)

	// Verifier with token issuer regex - invalid.
	verifier, err = NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", "test2"}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(token)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_Expired(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtExpired)
	require.Error(t, err)
	require.Equal(t, ErrTokenExpired, err)
}

func Test_tokenVerifierJWT_DisabledAlgorithm(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtExpired)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidToken), err.Error())
}

func Test_tokenVerifierJWT_InvalidSignature(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtInvalidSignature)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_WithNotBefore(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	_, err = verifier.VerifyConnectToken(jwtNotBefore)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_StringAudience(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtStringAud)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_ArrayAudience(t *testing.T) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)
	ct, err := verifier.VerifyConnectToken(jwtArrayAud)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_VerifyConnectToken(t *testing.T) {
	type args struct {
		token string
	}

	rsaPrivateKey, rsaPubKey := generateTestRSAKeys(t)
	ecdsaPrivateKey, ecdsaPubKey := generateTestECDSAKeys(t)

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)

	verifierJWT, err := NewTokenVerifierJWT(VerifierConfig{"secret", rsaPubKey, ecdsaPubKey, "", "", "", "", ""}, ruleContainer)
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
			got, err := tt.verifier.VerifyConnectToken(tt.args.token)
			if tt.wantErr && err == nil {
				t.Errorf("VerifyConnectToken() should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("VerifyConnectToken() should not return error")
			}
			if tt.expired && err != ErrTokenExpired {
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

			ruleConfig := rule.DefaultConfig
			ruleContainer, err := rule.NewContainer(ruleConfig)
			require.NoError(t, err)

			verifier, err := NewTokenVerifierJWT(VerifierConfig{"", nil, nil, ts.URL, "", "", "", ""}, ruleContainer)
			require.NoError(t, err)

			token := getRSAConnToken(tt.token.user, tt.token.exp, privKey, jwt.WithKeyID(tt.jwk.kid))

			got, err := verifier.VerifyConnectToken(token)
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

	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(t, err)

	verifierJWT, err := NewTokenVerifierJWT(VerifierConfig{"secret", rsaPubKey, ecdsaPubKey, "", "", "", "", ""}, ruleContainer)
	require.NoError(t, err)

	_time := time.Now()
	tests := []struct {
		name     string
		verifier Verifier
		args     args
		want     SubscribeToken
		wantErr  bool
		expired  bool
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
			got, err := tt.verifier.VerifySubscribeToken(tt.args.token)
			if tt.wantErr && err == nil {
				t.Errorf("VerifySubscribeToken() should return error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("VerifySubscribeToken() should not return error")
			}
			if tt.expired && err != ErrTokenExpired {
				t.Errorf("VerifySubscribeToken() should return token expired error")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VerifySubscribeToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkConnectTokenVerify_Valid(b *testing.B) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(b, err)
	verifierJWT, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := verifierJWT.VerifyConnectToken(jwtValid)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportAllocs()
}

func BenchmarkConnectTokenVerify_Expired(b *testing.B) {
	ruleConfig := rule.DefaultConfig
	ruleContainer, err := rule.NewContainer(ruleConfig)
	require.NoError(b, err)
	verifier, err := NewTokenVerifierJWT(VerifierConfig{"secret", nil, nil, "", "", "", "", ""}, ruleContainer)
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		_, err := verifier.VerifyConnectToken(jwtExpired)
		if err != ErrTokenExpired {
			panic(err)
		}
	}
}
