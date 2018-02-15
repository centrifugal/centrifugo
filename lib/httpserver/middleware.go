package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"
)

// APIKeyAuth protects endpoint by API key.
func APIKeyAuth(key string, n *node.Node, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")

		if key == "" {
			n.Logger().Log(logging.NewEntry(logging.ERROR, "API key is not configured"))
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

// LogRequest middleware logs request.
func LogRequest(n *node.Node, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start time.Time
		if n.Logger().Enabled(logging.DEBUG) {
			start = time.Now()
		}
		h.ServeHTTP(w, r)
		if n.Logger().Enabled(logging.DEBUG) {
			addr := r.Header.Get("X-Real-IP")
			if addr == "" {
				addr = r.Header.Get("X-Forwarded-For")
				if addr == "" {
					addr = r.RemoteAddr
				}
			}
			n.Logger().Log(logging.NewEntry(logging.DEBUG, fmt.Sprintf("%s %s from %s completed in %s", r.Method, r.URL.Path, addr, time.Since(start))))
		}
		return
	})
}
