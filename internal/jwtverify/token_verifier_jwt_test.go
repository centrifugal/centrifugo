package jwtverify

import (
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

	"github.com/cristalhq/jwt/v3"
	"github.com/stretchr/testify/require"
)

// Use https://jwt.io to look at token contents.
//noinspection ALL
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

func getTokenBuilder(rsaPrivateKey *rsa.PrivateKey, opts ...jwt.BuilderOption) *jwt.Builder {
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

func getConnToken(user string, exp int64, rsaPrivateKey *rsa.PrivateKey, opts ...jwt.BuilderOption) string {
	builder := getTokenBuilder(rsaPrivateKey, opts...)
	claims := &ConnectTokenClaims{
		Base64Info: "e30=",
		StandardClaims: jwt.StandardClaims{
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
	return string(token.Raw())
}

func getSubscribeToken(channel string, client string, exp int64, rsaPrivateKey *rsa.PrivateKey) string {
	builder := getTokenBuilder(rsaPrivateKey)
	claims := &SubscribeTokenClaims{
		Base64Info:     "e30=",
		Channel:        channel,
		Client:         client,
		StandardClaims: jwt.StandardClaims{},
	}
	if exp > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Unix(exp, 0))
	}
	token, err := builder.Build(claims)
	if err != nil {
		panic(err)
	}
	return string(token.Raw())
}

func getJWKServer(pubKey *rsa.PublicKey) *httptest.Server {
	return httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"keys": []map[string]string{
				{
					"alg": "RS256",
					"kty": "RSA",
					"use": "sig",
					"kid": "ok",
					"n":   encodeToString(pubKey.N.Bytes()),
					"e":   encodeUint64ToString(uint64(pubKey.E)),
				},
				{
					"alg": "RS256",
					"kty": "RSA",
					"use": "enc",
					"kid": "invalidkeyusage",
					"n":   encodeToString(pubKey.N.Bytes()),
					"e":   encodeUint64ToString(uint64(pubKey.E)),
				},
				{
					"alg": "HS256",
					"kty": "HS",
					"use": "sig",
					"kid": "invalidalgorithm",
					"n":   encodeToString(pubKey.N.Bytes()),
					"e":   encodeUint64ToString(uint64(pubKey.E)),
				},
			}}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func Test_tokenVerifierJWT_Signer(t *testing.T) {
	_, pubKey := generateTestRSAKeys(t)
	signer, err := newAlgorithms("secret", pubKey)
	require.NoError(t, err)
	require.NotNil(t, signer)
}

func Test_tokenVerifierJWT_Valid(t *testing.T) {
	verifier := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
	ct, err := verifier.VerifyConnectToken(jwtValid)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
	require.NotNil(t, ct.Info)
	require.Equal(t, `{"first_name":"Alexander","last_name":"Emelin"}`, string(ct.Info))
}

func Test_tokenVerifierJWT_Expired(t *testing.T) {
	verifier := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
	_, err := verifier.VerifyConnectToken(jwtExpired)
	require.Error(t, err)
	require.Equal(t, ErrTokenExpired, err)
}

func Test_tokenVerifierJWT_DisabledAlgorithm(t *testing.T) {
	verifier := NewTokenVerifierJWT(VerifierConfig{"", nil, ""})
	_, err := verifier.VerifyConnectToken(jwtExpired)
	require.Error(t, err)
	require.True(t, errors.Is(err, errDisabledAlgorithm), err.Error())
}

func Test_tokenVerifierJWT_InvalidSignature(t *testing.T) {
	verifier := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
	_, err := verifier.VerifyConnectToken(jwtInvalidSignature)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_WithNotBefore(t *testing.T) {
	verifier := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
	_, err := verifier.VerifyConnectToken(jwtNotBefore)
	require.Error(t, err)
}

func Test_tokenVerifierJWT_StringAudience(t *testing.T) {
	verifier := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
	ct, err := verifier.VerifyConnectToken(jwtStringAud)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_ArrayAudience(t *testing.T) {
	verifier := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
	ct, err := verifier.VerifyConnectToken(jwtArrayAud)
	require.NoError(t, err)
	require.Equal(t, "2694", ct.UserID)
}

func Test_tokenVerifierJWT_VerifyConnectToken(t *testing.T) {
	type args struct {
		token string
	}

	privateKey, pubKey := generateTestRSAKeys(t)

	verifierJWT := NewTokenVerifierJWT(VerifierConfig{"secret", pubKey, ""})
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
				token: getConnToken("user1", _time.Add(24*time.Hour).Unix(), nil),
			},
			want: ConnectToken{
				UserID:   "user1",
				ExpireAt: _time.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
			},
			wantErr: false,
		}, {
			name:     "Valid JWT RS",
			verifier: verifierJWT,
			args: args{
				token: getConnToken("user1", _time.Add(24*time.Hour).Unix(), privateKey),
			},
			want: ConnectToken{
				UserID:   "user1",
				ExpireAt: _time.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
			},
			wantErr: false,
		}, {
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
				token: getConnToken("user1", _time.Add(-24*time.Hour).Unix(), nil),
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VerifyConnectToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_tokenVerifierJWT_VerifyConnectTokenWithJWK(t *testing.T) {
	privKey, pubKey := generateTestRSAKeys(t)

	ts := getJWKServer(pubKey)
	defer ts.Close()

	verifierJWT := NewTokenVerifierJWT(VerifierConfig{"", nil, ts.URL})

	now := time.Now()

	testCases := []struct {
		name     string
		verifier Verifier
		token    string
		want     ConnectToken
		wantErr  bool
		expired  bool
	}{
		{
			name:     "OK",
			verifier: verifierJWT,
			token:    getConnToken("user1", now.Add(24*time.Hour).Unix(), privKey, jwt.WithKeyID("ok")),
			want: ConnectToken{
				UserID:   "user1",
				ExpireAt: now.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
			},
			wantErr: false,
		},
		{
			name:     "ExpiredToken",
			verifier: verifierJWT,
			token:    getConnToken("user1", now.Add(-24*time.Hour).Unix(), privKey, jwt.WithKeyID("ok")),
			want:     ConnectToken{},
			wantErr:  false,
			expired:  true,
		},
		{
			name:     "InvalidKeyUsage",
			verifier: verifierJWT,
			token:    getConnToken("user1", now.Add(24*time.Hour).Unix(), privKey, jwt.WithKeyID("invalidkeyusage")),
			want:     ConnectToken{},
			wantErr:  true,
		},
		{
			name:     "InvalidAlgorithm",
			verifier: verifierJWT,
			token:    getConnToken("user1", now.Add(24*time.Hour).Unix(), privKey, jwt.WithKeyID("invalidalgorithm")),
			want:     ConnectToken{},
			wantErr:  true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			r := require.New(t)

			got, err := tt.verifier.VerifyConnectToken(tt.token)
			if tt.wantErr {
				r.Error(err)

				if tt.expired {
					r.EqualError(err, ErrTokenExpired.Error())
				}

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

	privateKey, pubKey := generateTestRSAKeys(t)

	verifierJWT := NewTokenVerifierJWT(VerifierConfig{"secret", pubKey, ""})
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
				token: getSubscribeToken("channel1", "client1", _time.Add(-24*time.Hour).Unix(), nil),
			},
			want:    SubscribeToken{},
			wantErr: true,
			expired: true,
		}, {
			name:     "Valid JWT HS",
			verifier: verifierJWT,
			args: args{
				token: getSubscribeToken("channel1", "client1", _time.Add(24*time.Hour).Unix(), nil),
			},
			want: SubscribeToken{
				Client:   "client1",
				ExpireAt: _time.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
				Channel:  "channel1",
			},
			wantErr: false,
		}, {
			name:     "Valid JWT RS",
			verifier: verifierJWT,
			args: args{
				token: getSubscribeToken("channel1", "client1", _time.Add(24*time.Hour).Unix(), privateKey),
			},
			want: SubscribeToken{
				Client:   "client1",
				ExpireAt: _time.Add(24 * time.Hour).Unix(),
				Info:     []byte("{}"),
				Channel:  "channel1",
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
	verifierJWT := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
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
	verifier := NewTokenVerifierJWT(VerifierConfig{"secret", nil, ""})
	for i := 0; i < b.N; i++ {
		_, err := verifier.VerifyConnectToken(jwtExpired)
		if err != ErrTokenExpired {
			panic(err)
		}
	}
}
