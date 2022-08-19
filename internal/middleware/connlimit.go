package middleware

import (
	"net/http"

	"github.com/centrifugal/centrifugo/v4/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	connLimitReached prometheus.Counter
)

func init() {
	connLimitReached = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "centrifugo",
		Subsystem: "node",
		Name:      "client_connection_limit",
		Help:      "Number of refused requests due to node client connection limit.",
	})
	_ = prometheus.DefaultRegisterer.Register(connLimitReached)
}

func ConnLimit(node *centrifuge.Node, ruleContainer *rule.Container, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connLimit := ruleContainer.Config().ClientConnectionLimit
		if connLimit > 0 && node.Hub().NumClients() >= connLimit {
			connLimitReached.Inc()
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "node connection limit reached", map[string]interface{}{"limit": connLimit}))
			return
		}
		h.ServeHTTP(w, r)
	})
}
