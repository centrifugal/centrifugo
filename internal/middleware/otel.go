package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type OpenTelemetryHandler struct {
	operation string
	opts      []otelhttp.Option
}

func NewOpenTelemetryHandler(operation string, opts []otelhttp.Option) *OpenTelemetryHandler {
	return &OpenTelemetryHandler{operation: operation, opts: opts}
}

func (t *OpenTelemetryHandler) Middleware(h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, t.operation, t.opts...)
}
