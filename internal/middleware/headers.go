package middleware

import (
	"context"
	"net/http"
)

type contextHeadersKey struct{}

// HeadersFromContext returns http.Header from context.
func HeadersFromContext(ctx context.Context) http.Header {
	return ctx.Value(contextHeadersKey{}).(http.Header)
}

func headersToContext(ctx context.Context, h http.Header) context.Context {
	return context.WithValue(ctx, contextHeadersKey{}, h)
}

// HeadersToContext puts HTTP headers to request context.
func HeadersToContext(enable bool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !enable {
			h.ServeHTTP(w, r)
			return
		}
		r = r.WithContext(headersToContext(r.Context(), r.Header))
		h.ServeHTTP(w, r)
	})
}
