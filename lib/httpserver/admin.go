package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"

	"github.com/gorilla/securecookie"
)

// AdminConfig ...
type AdminConfig struct {
	// WebPath is path to admin web application to serve.
	WebPath string

	// WebFS is custom filesystem to serve as admin web application.
	WebFS http.FileSystem

	// Password is an admin password.
	Password string

	// Secret is a secret to generate auth token for admin requests.
	Secret string

	// Insecure turns on insecure mode for admin endpoints - no auth
	// required to connect to web interface and requests to admin API.
	// Protect admin resources with firewall rules in production when
	// enabling this option.
	Insecure bool
}

// AdminHandler handles admin web interface endpoints.
type AdminHandler struct {
	mux    *http.ServeMux
	node   *node.Node
	config AdminConfig
}

// NewAdminHandler creates new AdminHandler.
func NewAdminHandler(n *node.Node, c AdminConfig) *AdminHandler {
	h := &AdminHandler{
		node:   n,
		config: c,
	}
	mux := http.NewServeMux()
	mux.Handle("/admin/auth", http.HandlerFunc(h.authHandler))
	mux.Handle("/admin/api", h.adminSecureTokenAuth(NewAPIHandler(n, APIHandlerConfig{})))
	webPrefix := "/"
	if c.WebPath != "" {
		mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(http.Dir(c.WebPath))))
	} else if c.WebFS != nil {
		mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(c.WebFS)))
	}
	h.mux = mux
	return h
}

func (s *AdminHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(rw, r)
}

// adminSecureTokenAuth ...
func (s *AdminHandler) adminSecureTokenAuth(h http.Handler) http.Handler {

	secret := s.config.Secret
	insecure := s.config.Insecure

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
func (s *AdminHandler) authHandler(w http.ResponseWriter, r *http.Request) {
	formPassword := r.FormValue("password")

	insecure := s.config.Insecure
	password := s.config.Password
	secret := s.config.Secret

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

	if password == "" || secret == "" {
		s.node.Logger().Log(logging.NewEntry(logging.ERROR, "admin_password and admin_secret must be set in configuration"))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if formPassword == password {
		w.Header().Set("Content-Type", "application/json")
		token, err := generateSecureAdminToken(secret)
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
