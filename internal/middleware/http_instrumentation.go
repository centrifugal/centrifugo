package middleware

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequestsTotal *prometheus.CounterVec
)

func init() {
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "centrifugo",
			Subsystem: "node",
			Name:      "incoming_http_requests_total",
			Help:      "Number of incoming HTTP requests",
		},
		[]string{"path", "method", "status"},
	)
	_ = prometheus.DefaultRegisterer.Register(httpRequestsTotal)
}

// HTTPServerInstrumentation is a middleware to instrument HTTP handlers.
// Since it adds and extra layer of response wrapping Centrifugo doesn't use it by default.
// Note, we can not simply collect durations here because we have handlers with long-lived
// connections which require special care. So for now we just count requests.
func HTTPServerInstrumentation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusResponseWriter{w, http.StatusOK}
		next.ServeHTTP(rw, r)
		status := strconv.Itoa(rw.status)
		httpRequestsTotal.WithLabelValues(r.URL.Path, r.Method, status).Inc()
	})
}
