package middleware

import (
	"context"
	"net/http"
)

type contextHeadersKey struct{}

// GetHeadersFromContext returns http.Header from context.
func GetHeadersFromContext(ctx context.Context) (http.Header, bool) {
	if val := ctx.Value(contextHeadersKey{}); val != nil {
		values, ok := val.(http.Header)
		return values, ok
	}
	return nil, false
}

func SetHeadersToContext(ctx context.Context, h http.Header) context.Context {
	return context.WithValue(ctx, contextHeadersKey{}, h)
}

// HeadersToContext puts HTTP headers to request context.
func HeadersToContext(enable bool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enable {
			h.ServeHTTP(w, r)
			return
		}
		r = r.WithContext(SetHeadersToContext(r.Context(), r.Header))
		h.ServeHTTP(w, r)
	})
}
