package middleware

import (
	"net/http"

	"github.com/centrifugal/centrifuge"
)

// UserHeaderAuth is a middleware that extracts the value of user ID from the specific header
// and sets connection credentials.
func UserHeaderAuth(userHeaderName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := r.Header.Get(userHeaderName)
			if userID != "" {
				ctx := centrifuge.SetCredentials(r.Context(), &centrifuge.Credentials{
					UserID: userID,
				})
				r = r.WithContext(ctx)
			}
			// Call the next handler
			next.ServeHTTP(w, r)
		})
	}
}
