package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/lib/logging"
)

func (s *HTTPServer) apiAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		s.RLock()
		apiKey := s.config.APIKey
		apiInsecure := s.config.APIInsecure
		s.RUnlock()
		if apiKey == "" && !apiInsecure {
			s.node.Logger().Log(logging.NewEntry(logging.DEBUG, "no API key found in configuration"))
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

// checkAdminAuthToken checks admin connection token which Centrifugo returns after admin login.
func (s *HTTPServer) checkAdminAuthToken(token string) error {

	s.RLock()
	secret := s.config.AdminSecret
	insecure := s.config.AdminInsecure
	s.RUnlock()

	if insecure {
		return nil
	}

	if secret == "" {
		return fmt.Errorf("no admin secret set in configuration")
	}

	if token == "" {
		return fmt.Errorf("empty admin token")
	}

	auth := CheckAdminToken(secret, token)
	if !auth {
		return fmt.Errorf("wrong admin token")
	}
	return nil
}

func (s *HTTPServer) adminAPIAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")

		parts := strings.Fields(authorization)
		if len(parts) != 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authMethod := strings.ToLower(parts[0])
		if authMethod != "token" || s.checkAdminAuthToken(parts[1]) != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// wrapShutdown will return http Handler.
// If Application in shutdown it will return http.StatusServiceUnavailable.
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
		if s.node.Logger().GetLevel() >= logging.DEBUG {
			start = time.Now()
		}
		h.ServeHTTP(w, r)
		if s.node.Logger().GetLevel() >= logging.DEBUG {
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
