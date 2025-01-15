package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog/log"
)

// LogRequest middleware logs details of request.
func LogRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &statusResponseWriter{w, http.StatusOK}
		h.ServeHTTP(lrw, r)
		addr := r.Header.Get("X-Real-IP")
		if addr == "" {
			addr = r.Header.Get("X-Forwarded-For")
			if addr == "" {
				addr = r.RemoteAddr
			}
		}
		log.Debug().Str("method", r.Method).Int("status", lrw.Status()).Str("path", r.URL.Path).Str("addr", addr).Str("duration", time.Since(start).String()).Msg("http request")
	})
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader allows us to save status code.
func (lrw *statusResponseWriter) WriteHeader(status int) {
	lrw.status = status
	lrw.ResponseWriter.WriteHeader(status)
}

// Status code allows to get saved status code after handler finished its work.
func (lrw *statusResponseWriter) Status() int {
	if lrw.status == 0 {
		return http.StatusOK
	}
	return lrw.status
}

// Hijack as we need it for Websocket.
func (lrw *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	lrw.status = http.StatusSwitchingProtocols
	hijacker, ok := lrw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("ResponseWriter doesn't support Hijacker interface")
	}
	return hijacker.Hijack()
}

// Flush implements http.Flusher.
func (lrw *statusResponseWriter) Flush() {
	lrw.ResponseWriter.(http.Flusher).Flush()
}

// Connection for WebTransport.
func (lrw *statusResponseWriter) Connection() http3.Connection {
	return lrw.ResponseWriter.(http3.Hijacker).Connection()
}

// HTTPStream for WebTransport.
func (lrw *statusResponseWriter) HTTPStream() http3.Stream {
	return lrw.ResponseWriter.(http3.HTTPStreamer).HTTPStream()
}
