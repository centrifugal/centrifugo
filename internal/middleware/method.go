package middleware

import (
	"net/http"
)

// Post checks that handler called via POST HTTP method.
func Post(h http.Handler) http.Handler {
	return Method(http.MethodPost, h)
}

func Method(method string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		h.ServeHTTP(w, r)
	})
}
