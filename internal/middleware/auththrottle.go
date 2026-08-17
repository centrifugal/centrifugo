package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AuthThrottle limits repeated failing requests per client IP within a time
// window. It is meant for authentication-style endpoints (admin password auth,
// and any other endpoint where a caller repeatedly retries credentials) to slow
// brute-force without ever throttling a caller that succeeds.
//
// Only failures are counted, and the per-IP limit is checked before the wrapped
// handler runs, which gives two properties that matter for auth endpoints:
//
//   - A caller presenting valid credentials is never throttled, even while a
//     different source is actively brute-forcing: success does not accrue against
//     the limit, and each source IP is counted independently.
//   - Once an IP is over the limit its requests are rejected before the handler
//     runs, so the response cannot reveal whether the submitted credentials were
//     valid.
//
// It is safe for concurrent use. Wrap a handler with Middleware:
//
//	throttle := middleware.NewAuthThrottle(10, time.Minute, nil)
//	mux.Handle(path, throttle.Middleware(handler))
type AuthThrottle struct {
	max       int
	window    time.Duration
	isFailure func(status int) bool

	mu        sync.Mutex
	failures  map[string]int
	windowEnd time.Time
}

// authThrottleMapCap bounds the number of tracked IPs so a flood of requests with
// varying source addresses cannot grow the map without limit.
const authThrottleMapCap = 10000

// NewAuthThrottle creates an AuthThrottle allowing at most max failing requests
// per client IP within window, after which further requests from that IP are
// rejected with 429 until the window rolls over. isFailure decides, from the
// status the wrapped handler wrote, whether a request counts as a failure; if
// nil, any status >= 400 counts.
func NewAuthThrottle(max int, window time.Duration, isFailure func(status int) bool) *AuthThrottle {
	if isFailure == nil {
		isFailure = func(status int) bool { return status >= 400 }
	}
	return &AuthThrottle{
		max:       max,
		window:    window,
		isFailure: isFailure,
		failures:  make(map[string]int),
	}
}

// Middleware wraps h with per-IP failure throttling.
func (t *AuthThrottle) Middleware(h http.Handler) http.Handler {
	retryAfter := strconv.Itoa(int(t.window.Seconds()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !t.allow(ip) {
			w.Header().Set("Retry-After", retryAfter)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		sw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(sw, r)
		if t.isFailure(sw.Status()) {
			t.recordFailure(ip)
		}
	})
}

// allow reports whether another request from ip may reach the handler. It rolls
// the window over, discarding accumulated counts once the window elapses.
func (t *AuthThrottle) allow(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now := time.Now(); now.After(t.windowEnd) {
		t.failures = make(map[string]int)
		t.windowEnd = now.Add(t.window)
	}
	return t.failures[ip] < t.max
}

// recordFailure counts one failed request from ip. New IPs are not tracked once
// the map is at capacity, keeping memory bounded under an address-varying flood.
func (t *AuthThrottle) recordFailure(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, tracked := t.failures[ip]; !tracked && len(t.failures) >= authThrottleMapCap {
		return
	}
	t.failures[ip]++
}

// clientIP derives the client address used as the throttle key.
//
// Forwarded headers are only trusted when the immediate socket peer is a private
// or loopback address, i.e. a local reverse proxy or load balancer (Centrifugo is
// commonly deployed behind one). For a direct public client the headers are
// client-controlled and ignored, so a peer cannot spoof them to evade throttling
// or lock out another IP. Header values are validated and canonicalized as IPs
// before use, so junk cannot inflate map keys or fragment the keyspace.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil || (!peer.IsLoopback() && !peer.IsPrivate()) {
		return host
	}
	// Peer is a trusted local proxy: use the forwarded client address.
	if ip := parseIP(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			fwd = fwd[:i]
		}
		if ip := parseIP(strings.TrimSpace(fwd)); ip != "" {
			return ip
		}
	}
	return host
}

// parseIP returns the canonical string form of s if it is a valid IP, else "".
func parseIP(s string) string {
	if s == "" {
		return ""
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return ""
}
