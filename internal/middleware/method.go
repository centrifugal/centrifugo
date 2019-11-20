package middleware

import (
	"net/http"
)

// Post checks that handler called via POST HTTP method.
func Post(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		h.ServeHTTP(w, r)
	})
}
