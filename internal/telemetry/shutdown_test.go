package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// Spans still buffered when tracing shuts down are exported, and the globals
// set by SetupTracing are reset.
func TestShutdownTracingExportsAndResetsGlobals(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", server.URL)

	provider, err := SetupTracing(context.Background(), configtypes.OpenTelemetry{Enabled: true}, "node-1")
	require.NoError(t, err)
	require.Equal(t, provider, otel.GetTracerProvider())

	_, span := otel.Tracer("test").Start(context.Background(), "span")
	span.End()
	require.Zero(t, requests.Load()) // Batched, not exported yet.

	require.NoError(t, ShutdownTracing(context.Background(), provider))
	require.Positive(t, requests.Load())
	require.IsType(t, noop.TracerProvider{}, otel.GetTracerProvider())
	require.Empty(t, otel.GetTextMapPropagator().Fields())
}
