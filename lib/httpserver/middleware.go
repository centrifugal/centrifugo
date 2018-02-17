package httpserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/centrifugal/centrifugo/lib/logging"
	"github.com/centrifugal/centrifugo/lib/node"
)

// LogRequest middleware logs details of request.
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
