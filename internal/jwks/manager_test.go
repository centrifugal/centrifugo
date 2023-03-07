package jwks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rakutentech/jwk-go/jwk"
	"github.com/stretchr/testify/require"
)

type testKey struct {
	Kid string
	Key interface{}
}

func randomKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 512)
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

	manager, err := NewManager(server.URL+path, "")
	require.NoError(t, err)

	_, err = manager.FetchKey(context.Background(), "202101", "")
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

	manager, err := NewManager(server.URL+path, "")
	require.NoError(t, err)

	_, err = manager.FetchKey(context.Background(), "202101", "")
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

	manager, err := NewManager(server.URL+path, "")
	require.NoError(t, err)

	_, err = manager.FetchKey(context.Background(), "202101", "")
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

			manager, err := NewManager(ts.URL, "")
			r.NoError(err)

			key, err := manager.FetchKey(context.Background(), tc.Kid, "")
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

			manager, err := NewManager(ts.URL, "", tc.Options...)
			r.NoError(err)

			key, err := manager.FetchKey(ctx, kid, "")
			r.NoError(err)
			r.Equal(kid, key.Kid)

			size, err := manager.cache.Len()
			r.NoError(err)
			r.Equal(tc.ExpectedSize, size)
		})
	}
}

func Test_keycloakPublicKeyEndpoint(t *testing.T) {
	type args struct {
		keycloakBaseURL string
		iss             string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "invalid issuer",
			args: args{
				keycloakBaseURL: "https://example.com",
				iss:             "wrong",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "invalid issuer no path",
			args: args{
				keycloakBaseURL: "https://example.com",
				iss:             "https://example.com",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "invalid issuer no realm",
			args: args{
				keycloakBaseURL: "https://example.com",
				iss:             "https://example.com/auth/realms/",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "valid issuer",
			args: args{
				keycloakBaseURL: "https://example.com",
				iss:             "https://example.com/auth/realms/test",
			},
			want:    "https://example.com/test/protocol/openid-connect/certs",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := keycloakPublicKeyEndpoint(tt.args.keycloakBaseURL, tt.args.iss)
			if (err != nil) != tt.wantErr {
				t.Errorf("keycloakPublicKeyEndpoint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("keycloakPublicKeyEndpoint() got = %v, want %v", got, tt.want)
			}
		})
	}
}
