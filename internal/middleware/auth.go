package middleware

import (
	"net/http"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/rs/zerolog/log"
)

// APIKeyAuth middleware authorizes request using API key authorization.
// It first tries to use Authorization header to extract API key
// (Authorization: apikey <KEY>), then checks for api_key URL query parameter.
// If key not found or invalid then 401 response code is returned.
type APIKeyAuth struct {
	keys []string
}

func NewAPIKeyAuth(key string) *APIKeyAuth {
	keys := strings.Split(key, ",")
	return &APIKeyAuth{keys: keys}
}

func (a *APIKeyAuth) Middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(a.keys) == 0 {
			log.Error().Msg("API key is empty")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authValid := false
		authHeaderValue := r.Header.Get("X-API-Key")
		if authHeaderValue != "" {
			for _, key := range a.keys {
				if tools.SecureCompareString(key, authHeaderValue) {
					authValid = true
					break
				}
			}
		} else {
			authHeaderAuthorization := r.Header.Get("Authorization")
			if authHeaderAuthorization != "" {
				parts := strings.Fields(authHeaderAuthorization)
				if len(parts) == 2 && strings.ToLower(parts[0]) == "apikey" {
					for _, key := range a.keys {
						if tools.SecureCompareString(key, parts[1]) {
							authValid = true
							break
						}
					}
				}
			}
		}
		if !authValid && r.URL.RawQuery != "" {
			// Check URL param.
			for _, key := range a.keys {
				if tools.SecureCompareString(key, r.URL.Query().Get("api_key")) {
					authValid = true
					break
				}
			}
		}
		if !authValid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}
