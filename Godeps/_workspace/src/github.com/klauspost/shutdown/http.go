package shutdown

import (
	"net/http"
)

// WrapHandler will return an http Handler
// That will lock shutdown until all have completed
// and will return http.StatusServiceUnavailable if
// shutdown has been initiated.
func WrapHandler(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !Lock() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		h.ServeHTTP(w, r)
		Unlock()
	}
	return http.HandlerFunc(fn)
}
