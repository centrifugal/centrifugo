package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// goodPassword is accepted by the test handler; anything else returns 400.
const goodPassword = "correct"

func throttleTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pw") == goodPassword {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "bad", http.StatusBadRequest)
	})
}

func doReq(h http.Handler, ip, pw string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/auth?pw="+pw, nil)
	r.RemoteAddr = "10.0.0.1:12345" // trusted local proxy peer, so X-Real-IP is honored.
	r.Header.Set("X-Real-IP", ip)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAuthThrottle_LimitsFailuresPerIP(t *testing.T) {
	h := NewAuthThrottle(3, time.Minute, nil).Middleware(throttleTestHandler())

	// First 3 wrong attempts from an IP reach the handler and return its status.
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusBadRequest, doReq(h, "1.1.1.1", "wrong").Code, "attempt %d", i)
	}
	// The 4th is rejected before the handler runs, with a Retry-After hint.
	resp := doReq(h, "1.1.1.1", "wrong")
	require.Equal(t, http.StatusTooManyRequests, resp.Code)
	require.Equal(t, strconv.Itoa(60), resp.Header().Get("Retry-After"))

	// Even a correct password from the throttled IP is rejected with 429, so the
	// response does not reveal that the credentials were valid.
	require.Equal(t, http.StatusTooManyRequests, doReq(h, "1.1.1.1", goodPassword).Code)
}

func TestAuthThrottle_DoesNotBlockOtherIPsOrSuccess(t *testing.T) {
	h := NewAuthThrottle(3, time.Minute, nil).Middleware(throttleTestHandler())

	// Exhaust the limit for the attacker's IP.
	for i := 0; i < 5; i++ {
		_ = doReq(h, "9.9.9.9", "wrong")
	}
	require.Equal(t, http.StatusTooManyRequests, doReq(h, "9.9.9.9", "wrong").Code)

	// A legitimate admin on a different IP still logs in while the attack runs.
	require.Equal(t, http.StatusOK, doReq(h, "2.2.2.2", goodPassword).Code)

	// Success never accrues against the limit: many valid logins stay allowed.
	for i := 0; i < 20; i++ {
		require.Equal(t, http.StatusOK, doReq(h, "3.3.3.3", goodPassword).Code, "login %d", i)
	}
}

func TestAuthThrottle_PublicPeerCannotSpoofHeader(t *testing.T) {
	h := NewAuthThrottle(3, time.Minute, nil).Middleware(throttleTestHandler())
	// A direct public client rotating X-Real-IP cannot evade the limit: the header
	// is untrusted (peer is not a local proxy), so every attempt keys on the real
	// socket peer and the IP is throttled after the limit regardless of the header.
	req := func(spoofedIP string) int {
		r := httptest.NewRequest(http.MethodPost, "/auth?pw=wrong", nil)
		r.RemoteAddr = "203.0.113.5:9999" // public peer.
		r.Header.Set("X-Real-IP", spoofedIP)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	require.Equal(t, http.StatusBadRequest, req("1.1.1.1"))
	require.Equal(t, http.StatusBadRequest, req("2.2.2.2"))
	require.Equal(t, http.StatusBadRequest, req("3.3.3.3"))
	require.Equal(t, http.StatusTooManyRequests, req("4.4.4.4"))
}

func TestAuthThrottle_WindowReset(t *testing.T) {
	h := NewAuthThrottle(2, 50*time.Millisecond, nil).Middleware(throttleTestHandler())

	require.Equal(t, http.StatusBadRequest, doReq(h, "1.2.3.4", "wrong").Code)
	require.Equal(t, http.StatusBadRequest, doReq(h, "1.2.3.4", "wrong").Code)
	require.Equal(t, http.StatusTooManyRequests, doReq(h, "1.2.3.4", "wrong").Code)

	// After the window elapses the count resets and the IP may try again.
	time.Sleep(70 * time.Millisecond)
	require.Equal(t, http.StatusBadRequest, doReq(h, "1.2.3.4", "wrong").Code)
}

func TestAuthThrottle_MapBounded(t *testing.T) {
	tr := NewAuthThrottle(1, time.Minute, nil)
	// Track an IP, then flood the map with unique addresses past its capacity.
	tr.recordFailure("10.0.0.1")
	for i := 0; i < authThrottleMapCap+100; i++ {
		tr.recordFailure("10.9." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256))
	}
	// The already-tracked IP keeps counting even though the map is now full.
	tr.recordFailure("10.0.0.1")

	tr.mu.Lock()
	size := len(tr.failures)
	pretrackedCount := tr.failures["10.0.0.1"]
	tr.mu.Unlock()
	require.LessOrEqual(t, size, authThrottleMapCap, "map must stay bounded")
	require.Equal(t, 2, pretrackedCount, "already-tracked IP keeps counting")
}

func TestClientIP(t *testing.T) {
	newReq := func(realIP, xff, remote string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = remote
		if realIP != "" {
			r.Header.Set("X-Real-IP", realIP)
		}
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	// Trusted local proxy peer (private / loopback): forwarded headers are used.
	require.Equal(t, "5.5.5.5", clientIP(newReq("5.5.5.5", "1.1.1.1", "10.0.0.9:1")))
	require.Equal(t, "1.1.1.1", clientIP(newReq("", "1.1.1.1, 2.2.2.2", "10.0.0.9:1")))
	require.Equal(t, "8.8.8.8", clientIP(newReq("8.8.8.8", "", "127.0.0.1:1")))

	// Direct public peer: forwarded headers are client-controlled, so ignored -
	// the socket peer is used and a spoofed header cannot change the key.
	require.Equal(t, "9.9.9.9", clientIP(newReq("", "", "9.9.9.9:12345")))
	require.Equal(t, "9.9.9.9", clientIP(newReq("1.2.3.4", "1.2.3.4", "9.9.9.9:12345")))

	// Behind a trusted proxy, a non-IP header value falls back to the peer, so an
	// attacker cannot inflate map keys or fragment the keyspace with junk.
	require.Equal(t, "10.0.0.9", clientIP(newReq(strings.Repeat("x", 5000), "", "10.0.0.9:1")))
	require.Equal(t, "10.0.0.9", clientIP(newReq("", "not-an-ip", "10.0.0.9:1")))

	require.Equal(t, "raw-addr", clientIP(newReq("", "", "raw-addr")))
}
