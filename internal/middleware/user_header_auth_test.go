package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func userHeaderAuthTestHandler(t *testing.T, userMustBeSet bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cred, ok := centrifuge.GetCredentials(r.Context())
		if userMustBeSet {
			require.True(t, ok, "credentials should be set")
			require.Equal(t, "123", cred.UserID, "user ID should be set correctly")
		} else {
			require.False(t, ok)
		}
		_, _ = w.Write([]byte("OK"))
	})
}

func TestUserHeaderAuthWithUserID(t *testing.T) {
	middleware := UserHeaderAuth("X-User-ID")
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-ID", "123")
	rr := httptest.NewRecorder()

	handler := middleware(userHeaderAuthTestHandler(t, true))
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "status code should be 200 OK")
}

func TestUserHeaderAuthWithoutUserID(t *testing.T) {
	middleware := UserHeaderAuth("X-User-ID")
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler := middleware(userHeaderAuthTestHandler(t, false))
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "status code should be 200 OK")
}
