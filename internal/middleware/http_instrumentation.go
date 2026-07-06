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
		// Use the matched route pattern, not the raw request path. Subtree
		// handlers (admin, swagger, dev, debug) accept arbitrary client-controlled
		// sub-paths, so labeling by r.URL.Path would grow the Prometheus series map
		// without bound (memory DoS). r.Pattern is the registered pattern that
		// matched (e.g. "/admin/"), so cardinality is bounded by the route count.
		path := r.Pattern
		if path == "" {
			path = "other"
		}
		metrics.HTTPRequestsTotal.WithLabelValues(path, r.Method, status).Inc()
	})
}
