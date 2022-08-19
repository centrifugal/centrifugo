package middleware

import (
	"net/http"

	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/v4/internal/rule"
)

func ConnLimit(node *centrifuge.Node, ruleContainer *rule.Container, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connLimit := ruleContainer.Config().ClientConnectionLimit
		if connLimit > 0 && node.Hub().NumClients() >= connLimit {
			node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelWarn, "node connection limit reached", map[string]interface{}{"limit": connLimit}))
			return
		}
		h.ServeHTTP(w, r)
	})
}
