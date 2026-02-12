package middleware

import (
	"net/http"
	"strconv"

	"github.com/centrifugal/centrifugo/v6/internal/metrics"
)

// HTTPServerInstrumentation is a middleware to instrument HTTP handlers.
// Since it adds and extra layer of response wrapping Centrifugo doesn't use it by default.
// Note, we can not simply collect durations here because we have handlers with long-lived
// connections which require special care. So for now we just count requests.
func HTTPServerInstrumentation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusResponseWriter{w, http.StatusOK}
		next.ServeHTTP(rw, r)
		status := strconv.Itoa(rw.status)
		metrics.HTTPRequestsTotal.WithLabelValues(r.URL.Path, r.Method, status).Inc()
	})
}
