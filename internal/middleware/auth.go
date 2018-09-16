package middleware

import (
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// APIKeyAuth middleware authorizes request using API key authorization.
func APIKeyAuth(key string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key == "" {
			log.Error().Msg("API key is empty")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authorization := r.Header.Get("Authorization")
		parts := strings.Fields(authorization)
		if len(parts) != 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authMethod := strings.ToLower(parts[0])
		if authMethod != "apikey" || parts[1] != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}
