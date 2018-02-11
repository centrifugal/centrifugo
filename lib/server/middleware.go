package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/lib/logging"
)

// apiKeyAuth protects endpoint by API key.
func (s *HTTPServer) apiKeyAuth(h http.Handler) http.Handler {

	s.RLock()
	apiKey := s.config.APIKey
	apiInsecure := s.config.APIInsecure
	s.RUnlock()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")

		if apiKey == "" && !apiInsecure {
			s.node.Logger().Log(logging.NewEntry(logging.ERROR, "no API key found in configuration"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !apiInsecure {
			parts := strings.Fields(authorization)
			if len(parts) != 2 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			authMethod := strings.ToLower(parts[0])
			if authMethod != "apikey" || parts[1] != apiKey {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// wrapShutdown will return http Handler.
// If Server in shutdown state it will return http.StatusServiceUnavailable.
func (s *HTTPServer) wrapShutdown(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.RLock()
		shutdown := s.shutdown
		s.RUnlock()
		if shutdown {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// log middleware logs request.
func (s *HTTPServer) log(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start time.Time
		if s.node.Logger().Enabled(logging.DEBUG) {
			start = time.Now()
		}
		h.ServeHTTP(w, r)
		if s.node.Logger().Enabled(logging.DEBUG) {
			addr := r.Header.Get("X-Real-IP")
			if addr == "" {
				addr = r.Header.Get("X-Forwarded-For")
				if addr == "" {
					addr = r.RemoteAddr
				}
			}
			s.node.Logger().Log(logging.NewEntry(logging.DEBUG, fmt.Sprintf("%s %s from %s completed in %s", r.Method, r.URL.Path, addr, time.Since(start))))
		}
		return
	})
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
