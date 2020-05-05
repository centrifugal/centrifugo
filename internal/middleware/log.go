package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LogRequest middleware logs details of request.
func LogRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if zerolog.GlobalLevel() <= zerolog.DebugLevel {
			start := time.Now()
			lrw := &logResponseWriter{w, 0}
			h.ServeHTTP(lrw, r)
			addr := r.Header.Get("X-Real-IP")
			if addr == "" {
				addr = r.Header.Get("X-Forwarded-For")
				if addr == "" {
					addr = r.RemoteAddr
				}
			}
			log.Debug().Str("method", r.Method).Int("status", lrw.Status()).Str("path", r.URL.Path).Str("addr", addr).Dur("duration", time.Since(start)).Msgf("http request")
		} else {
			h.ServeHTTP(w, r)
		}
		return
	})
}

type logResponseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader allows us to save status code.
func (lrw *logResponseWriter) WriteHeader(status int) {
	lrw.status = status
	lrw.ResponseWriter.WriteHeader(status)
}

// Status code allows to get saved status code after handler finished its work.
func (lrw *logResponseWriter) Status() int {
	if lrw.status == 0 {
		return http.StatusOK
	}
	return lrw.status
}

// Hijack as we need it for Websocket.
func (lrw *logResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	lrw.status = http.StatusSwitchingProtocols
	hijacker, ok := lrw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("ResponseWriter doesn't support Hijacker interface")
	}
	return hijacker.Hijack()
}

// Flush as SockJS uses http.Flusher.
func (lrw *logResponseWriter) Flush() {
	lrw.ResponseWriter.(http.Flusher).Flush()
}

// CloseNotify as SockJS uses http.CloseNotifier.
func (lrw *logResponseWriter) CloseNotify() <-chan bool {
	return lrw.ResponseWriter.(http.CloseNotifier).CloseNotify()
}
