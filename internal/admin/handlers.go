package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/centrifugal/centrifugo/internal/api"
	"github.com/centrifugal/centrifugo/internal/middleware"

	"github.com/centrifugal/centrifuge"
	"github.com/gorilla/securecookie"
	"github.com/rs/zerolog/log"
)

// Config ...
type Config struct {
	// Prefix is a custom prefix to handle admin endpoints on.
	Prefix string

	// WebPath is path to admin web application to serve.
	WebPath string

	// WebFS is custom filesystem to serve as admin web application.
	WebFS http.FileSystem

	// Password is an admin password.
	Password string

	// TokenHMACSecretKey is a secret to generate auth token for admin requests.
	Secret string

	// Insecure turns on insecure mode for admin endpoints - no auth
	// required to connect to web interface and requests to admin API.
	// Protect admin resources with firewall rules in production when
	// enabling this option.
	Insecure bool
}

// Handler handles admin web interface endpoints.
type Handler struct {
	mux    *http.ServeMux
	node   *centrifuge.Node
	config Config
}

// NewHandler creates new Handler.
func NewHandler(n *centrifuge.Node, c Config) *Handler {
	h := &Handler{
		node:   n,
		config: c,
	}
	mux := http.NewServeMux()
	prefix := strings.TrimRight(h.config.Prefix, "/")
	mux.Handle(prefix+"/admin/auth", middleware.Post(http.HandlerFunc(h.authHandler)))
	mux.Handle(prefix+"/admin/api", middleware.Post(h.adminSecureTokenAuth(api.NewHandler(n, api.Config{}))))
	webPrefix := prefix + "/"
	if c.WebPath != "" {
		mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(http.Dir(c.WebPath))))
	} else if c.WebFS != nil {
		mux.Handle(webPrefix, http.StripPrefix(webPrefix, http.FileServer(c.WebFS)))
	}
	h.mux = mux
	return h
}

func (s *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(rw, r)
}

// adminSecureTokenAuth ...
func (s *Handler) adminSecureTokenAuth(h http.Handler) http.Handler {

	secret := s.config.Secret
	insecure := s.config.Insecure

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if insecure {
			h.ServeHTTP(w, r)
			return
		}

		if secret == "" {
			log.Error().Msg("no admin secret key found in configuration")
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
func (s *Handler) authHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Error().Msg("admin_password and admin_secret must be set in configuration")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if formPassword == password {
		w.Header().Set("Content-Type", "application/json")
		token, err := generateSecureAdminToken(secret)
		if err != nil {
			log.Error().Msgf("error generating admin token: %v", err)
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
