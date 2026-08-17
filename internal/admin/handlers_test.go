package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

// TestNewHandler ensures NewHandler sets up routes and proxy handling correctly.
func TestNewHandler(t *testing.T) {
	node := &centrifuge.Node{}
	cfg := Config{HandlerPrefix: "/prefix", WebProxyAddress: "", WebPath: ""}
	handler := NewHandler(node, nil, cfg)
	require.NotNil(t, handler)
	require.NotNil(t, handler.mux)

	// Validate init route is registered.
	req := httptest.NewRequest("GET", "/prefix/admin/init", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
}

// TestInitHandler checks the initHandler returns correct settings.
func TestInitHandler(t *testing.T) {
	config := Config{}
	handler := &Handler{config: config}
	req := httptest.NewRequest("GET", "/admin/init", nil)
	resp := httptest.NewRecorder()

	handler.initHandler(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var response map[string]any
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.Equal(t, "oss", response["edition"])
	require.Equal(t, false, response["insecure"])
	require.Equal(t, false, response["authenticated"])
}

func TestInitHandler_Insecure(t *testing.T) {
	config := Config{Insecure: true}
	handler := &Handler{config: config}
	req := httptest.NewRequest("GET", "/admin/init", nil)
	resp := httptest.NewRecorder()

	handler.initHandler(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var response map[string]any
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.Equal(t, "oss", response["edition"])
	require.Equal(t, true, response["insecure"])
	require.Equal(t, true, response["authenticated"]) // Since insecure on.
}

// TestAuthHandler_NoPasswordOrSecret tests authHandler error when password or secret is missing.
func TestAuthHandler_NoPasswordOrSecret(t *testing.T) {
	config := Config{Password: "", Secret: ""}
	handler := &Handler{config: config}
	req := httptest.NewRequest("POST", "/admin/auth", nil)
	resp := httptest.NewRecorder()

	handler.authHandler(resp, req)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

// TestAuthHandler_ValidPassword tests authHandler token generation with valid password.
func TestAuthHandler_ValidPassword(t *testing.T) {
	config := Config{Password: "test-password", Secret: "test-secret"}
	handler := &Handler{config: config}
	form := url.Values{}
	form.Add("password", "test-password")
	req := httptest.NewRequest("POST", "/admin/auth", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	handler.authHandler(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var response map[string]string
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotEmpty(t, response["token"])
}

// TestAuthHandler_InvalidPassword tests authHandler rejection with invalid password.
func TestAuthHandler_InvalidPassword(t *testing.T) {
	config := Config{Password: "test-password", Secret: "test-secret"}
	handler := &Handler{config: config}
	form := url.Values{}
	form.Add("password", "wrong-password")
	req := httptest.NewRequest("POST", "/admin/auth", strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	handler.authHandler(resp, req)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

// TestAuthHandler_Throttled ensures the /admin/auth route throttles repeated
// failed password attempts per client IP, without blocking a valid login from
// another IP while an attacker is brute-forcing.
func TestAuthHandler_Throttled(t *testing.T) {
	node := &centrifuge.Node{}
	cfg := Config{Password: "test-password", Secret: "test-secret"}
	handler := NewHandler(node, nil, cfg)

	post := func(ip, password string) int {
		form := url.Values{}
		form.Add("password", password)
		req := httptest.NewRequest("POST", "/admin/auth", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "10.0.0.1:12345" // trusted local proxy peer, so X-Real-IP is honored.
		req.Header.Set("X-Real-IP", ip)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp.Code
	}

	// The default limit is 10 failures per IP per window.
	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusBadRequest, post("6.6.6.6", "wrong"), "attempt %d", i)
	}
	require.Equal(t, http.StatusTooManyRequests, post("6.6.6.6", "wrong"))
	// The attacker cannot tell a correct guess from a throttled one.
	require.Equal(t, http.StatusTooManyRequests, post("6.6.6.6", "test-password"))

	// A valid login from a different IP still succeeds during the attack.
	require.Equal(t, http.StatusOK, post("7.7.7.7", "test-password"))
}

// TestAdminSecureTokenAuth_InsecureMode tests adminSecureTokenAuth allows request in insecure mode.
func TestAdminSecureTokenAuth_InsecureMode(t *testing.T) {
	config := Config{Insecure: true}
	handler := &Handler{config: config}

	// Mocked handler that should be invoked
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authHandler := handler.adminSecureTokenAuth(finalHandler)
	req := httptest.NewRequest("GET", "/admin/api", nil)
	resp := httptest.NewRecorder()

	authHandler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
}

// TestAdminSecureTokenAuth_MissingToken tests adminSecureTokenAuth rejection when token is missing.
func TestAdminSecureTokenAuth_MissingToken(t *testing.T) {
	config := Config{Secret: "test-secret"}
	handler := &Handler{config: config}

	// Mocked handler that should not be invoked
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	authHandler := handler.adminSecureTokenAuth(finalHandler)
	req := httptest.NewRequest("GET", "/admin/api", nil)
	resp := httptest.NewRecorder()

	authHandler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

// TestAdminSecureTokenAuth_ValidToken tests adminSecureTokenAuth with valid token.
func TestAdminSecureTokenAuth_ValidToken(t *testing.T) {
	config := Config{Secret: "test-secret"}
	handler := &Handler{config: config}

	// Mocked handler that should be invoked
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Generate a valid token
	token, err := generateSecureAdminToken("test-secret")
	require.NoError(t, err)

	authHandler := handler.adminSecureTokenAuth(finalHandler)
	req := httptest.NewRequest("GET", "/admin/api", nil)
	req.Header.Set("Authorization", "token "+token)
	resp := httptest.NewRecorder()

	authHandler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
}

// TestAdminSecureTokenAuth_InvalidToken tests adminSecureTokenAuth rejection with invalid token.
func TestAdminSecureTokenAuth_InvalidToken(t *testing.T) {
	config := Config{Secret: "test-secret"}
	handler := &Handler{config: config}

	// Mocked handler that should not be invoked.
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Fail(t, "final handler should not be invoked")
		w.WriteHeader(http.StatusOK)
	})

	authHandler := handler.adminSecureTokenAuth(finalHandler)
	req := httptest.NewRequest("GET", "/admin/api", nil)
	req.Header.Set("Authorization", "token invalid-token")
	resp := httptest.NewRecorder()

	authHandler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}
