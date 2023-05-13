package middleware

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/rule"

	"github.com/centrifugal/centrifuge"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
)

var (
	connLimitReached         prometheus.Counter
	connLimitReachedLoggedAt int64
)

const connLimitReachedLogThrottle = int64(3 * time.Second)

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
	rl := connectionRateLimiter(ruleContainer.Config().ClientConnectionRateLimit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl != nil && !rl.Allow() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		connLimit := ruleContainer.Config().ClientConnectionLimit
		if connLimit > 0 && node.Hub().NumClients() >= connLimit {
			connLimitReached.Inc()
			now := time.Now().UnixNano()
			prevLoggedAt := atomic.LoadInt64(&connLimitReachedLoggedAt)
			if prevLoggedAt == 0 || now-prevLoggedAt > connLimitReachedLogThrottle {
				node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "node connection limit reached", map[string]interface{}{"limit": connLimit}))
				atomic.StoreInt64(&connLimitReachedLoggedAt, now)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func connectionRateLimiter(connRateLimit int) *rate.Limiter {
	if connRateLimit > 0 {
		return rate.NewLimiter(rate.Every(time.Second), connRateLimit)
	}
	return nil
}
