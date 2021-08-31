package middleware

import "net/http"

type OriginCheck func(r *http.Request) bool

// CORS middleware.
func CORS(originCheck OriginCheck, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		if originCheck(r) {
			header.Set("Access-Control-Allow-Origin", r.Header.Get("origin"))
			if allowHeaders := r.Header.Get("Access-Control-Request-Headers"); allowHeaders != "" && allowHeaders != "null" {
				header.Add("Access-Control-Allow-Headers", allowHeaders)
			}
			header.Set("Access-Control-Allow-Credentials", "true")
		}
		h.ServeHTTP(w, r)
	})
}
