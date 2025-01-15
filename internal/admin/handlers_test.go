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

	// Validate settings route is registered.
	req := httptest.NewRequest("GET", "/prefix/admin/settings", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
}

// TestSettingsHandler checks the settingsHandler returns correct settings.
func TestSettingsHandler(t *testing.T) {
	config := Config{Insecure: true}
	handler := &Handler{config: config}
	req := httptest.NewRequest("GET", "/admin/settings", nil)
	resp := httptest.NewRecorder()

	handler.settingsHandler(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)

	var response map[string]any
	err := json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.Equal(t, "oss", response["edition"])
	require.Equal(t, config.Insecure, response["insecure"])
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
	req := httptest.NewRequest("GET", "/admin/api?token="+token, nil)
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
	req := httptest.NewRequest("GET", "/admin/api?token=invalid-token", nil)
	resp := httptest.NewRecorder()

	authHandler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}
