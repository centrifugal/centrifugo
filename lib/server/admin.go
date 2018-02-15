package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/centrifugal/centrifugo/lib/logging"

	"github.com/gorilla/securecookie"
)

// AdminHandlerConfig ...
type AdminHandlerConfig struct {
	// AdminPassword is an admin password.
	AdminPassword string `json:"admin_password"`

	// AdminSecret is a secret to generate auth token for admin requests.
	AdminSecret string `json:"admin_secret"`

	// AdminInsecure turns on insecure mode for admin endpoints - no auth required to
	// connect to web interface and requests to admin API. Protect admin resources with
	// firewall rules in production when enabling this option.
	AdminInsecure bool `json:"admin_insecure"`
}

// adminSecureTokenAuth ...
func (s *HTTPServer) adminSecureTokenAuth(h http.Handler) http.Handler {

	s.RLock()
	secret := s.config.AdminSecret
	insecure := s.config.AdminInsecure
	s.RUnlock()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if insecure {
			h.ServeHTTP(w, r)
			return
		}

		if secret == "" {
			s.node.Logger().Log(logging.NewEntry(logging.ERROR, "no admin secret key found in configuration"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		authorization := r.Header.Get("Authorization")

		parts := strings.Fields(authorization)
		if len(parts) != 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authMethod := strings.ToLower(parts[0])

		if authMethod != "token" || !checkSecureAdminToken(secret, parts[1]) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// authHandler allows to get admin web interface token.
func (s *HTTPServer) authHandler(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")

	insecure := s.config.AdminInsecure
	adminPassword := s.config.AdminPassword
	adminSecret := s.config.AdminSecret

	if insecure {
		w.Header().Set("Content-Type", "application/json")
		resp := struct {
			Token string `json:"token"`
		}{
			Token: "insecure",
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	if adminPassword == "" || adminSecret == "" {
		s.node.Logger().Log(logging.NewEntry(logging.ERROR, "admin_password and admin_secret must be set in configuration"))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if password == adminPassword {
		w.Header().Set("Content-Type", "application/json")
		token, err := generateSecureAdminToken(adminSecret)
		if err != nil {
			s.node.Logger().Log(logging.NewEntry(logging.ERROR, "error generating admin token", map[string]interface{}{"error": err.Error()}))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		resp := map[string]string{
			"token": token,
		}
		json.NewEncoder(w).Encode(resp)
		return
	}
	http.Error(w, "Bad Request", http.StatusBadRequest)
}

const (
	// AdminTokenKey is a key for admin authorization token.
	secureAdminTokenKey = "token"
	// AdminTokenValue is a value for secure admin authorization token.
	secureAdminTokenValue = "authorized"
)

// generateSecureAdminToken generates admin authentication token.
func generateSecureAdminToken(secret string) (string, error) {
	s := securecookie.New([]byte(secret), nil)
	return s.Encode(secureAdminTokenKey, secureAdminTokenValue)
}

// checkSecureAdminToken checks admin connection token which Centrifugo returns after admin login.
func checkSecureAdminToken(secret string, token string) bool {
	s := securecookie.New([]byte(secret), nil)
	var val string
	err := s.Decode(secureAdminTokenKey, token, &val)
	if err != nil {
		return false
	}
	if val != secureAdminTokenValue {
		return false
	}
	return true
}
