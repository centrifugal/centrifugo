package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LogRequest middleware logs details of request.
func LogRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start time.Time
		if zerolog.GlobalLevel() <= zerolog.DebugLevel {
			start = time.Now()
		}
		h.ServeHTTP(w, r)
		if zerolog.GlobalLevel() <= zerolog.DebugLevel {
			addr := r.Header.Get("X-Real-IP")
			if addr == "" {
				addr = r.Header.Get("X-Forwarded-For")
				if addr == "" {
					addr = r.RemoteAddr
				}
			}
			log.Debug().Str("method", r.Method).Str("path", r.URL.Path).Str("addr", addr).Dur("duration", time.Since(start)).Msgf("http request")
		}
		return
	})
}
