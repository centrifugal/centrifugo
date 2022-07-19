package middleware

import (
	"net/http"
	"strings"

	"github.com/centrifugal/centrifugo/v3/internal/tools"

	"github.com/rs/zerolog/log"
)

// APIKeyAuth middleware authorizes request using API key authorization.
// It first tries to use Authorization header to extract API key
// (Authorization: apikey <KEY>), then checks for api_key URL query parameter.
// If key not found or invalid then 401 response code is returned.
func APIKeyAuth(key string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key == "" {
			log.Error().Msg("API key is empty")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authValid := false
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Fields(authHeader)
			if len(parts) == 2 && strings.ToLower(parts[0]) == "apikey" && tools.SecureCompareString(key, parts[1]) {
				authValid = true
			}
		}
		if !authValid && r.URL.RawQuery != "" {
			// Check URL param.
			if tools.SecureCompareString(key, r.URL.Query().Get("api_key")) {
				authValid = true
			}
		}
		if !authValid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}
