package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/lib/logging"
)

// APIKeyAuth protects endpoint by API key.
func (s *HTTPServer) APIKeyAuth(key string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")

		if key == "" {
			s.node.Logger().Log(logging.NewEntry(logging.ERROR, "API key is not configured"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		parts := strings.Fields(authorization)
		if len(parts) != 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authMethod := strings.ToLower(parts[0])
		if authMethod != "apikey" || parts[1] != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// WrapShutdown will return http.StatusServiceUnavailable if server in shutdown state.
func (s *HTTPServer) WrapShutdown(h http.Handler) http.Handler {
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

// LogRequest middleware logs request.
func (s *HTTPServer) LogRequest(h http.Handler) http.Handler {
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
