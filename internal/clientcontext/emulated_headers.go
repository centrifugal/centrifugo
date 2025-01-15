package clientcontext

import (
	"context"
)

type emulatedHeadersKey struct{}

// GetEmulatedHeadersFromContext returns emulated headers from context.
func GetEmulatedHeadersFromContext(ctx context.Context) (map[string]string, bool) {
	if val := ctx.Value(emulatedHeadersKey{}); val != nil {
		values, ok := val.(map[string]string)
		return values, ok
	}
	return nil, false
}

// SetEmulatedHeadersToContext sets header map to context.
func SetEmulatedHeadersToContext(ctx context.Context, h map[string]string) context.Context {
	if len(h) == 0 {
		return ctx
	}
	return context.WithValue(ctx, emulatedHeadersKey{}, h)
}
