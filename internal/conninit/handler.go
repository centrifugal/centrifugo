package conninit

import (
	"net/http"
)

// NewHandler creates a new HTTP handler for connection initialization endpoint.
// This endpoint can be used by clients to prepare HTTP/2 connection before WebSocket upgrade.
// Or as public endpoint for ALB health checks.
func NewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	})
}
