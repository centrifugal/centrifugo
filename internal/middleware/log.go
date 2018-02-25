package middleware

import (
	"net/http"
	"time"

	"github.com/FZambia/go-logger"
	"github.com/centrifugal/centrifuge"
)

// LogRequest middleware logs details of request.
func LogRequest(n *centrifuge.Node, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start time.Time
		if logger.DEBUG.Enabled() {
			start = time.Now()
		}
		h.ServeHTTP(w, r)
		if logger.DEBUG.Enabled() {
			addr := r.Header.Get("X-Real-IP")
			if addr == "" {
				addr = r.Header.Get("X-Forwarded-For")
				if addr == "" {
					addr = r.RemoteAddr
				}
			}
			logger.DEBUG.Printf("%s %s from %s completed in %s", r.Method, r.URL.Path, addr, time.Since(start))
		}
		return
	})
}
