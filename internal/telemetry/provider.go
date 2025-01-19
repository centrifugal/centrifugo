package telemetry

import (
	"context"
	"fmt"
	"os"

	"github.com/centrifugal/centrifugo/v6/internal/build"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

func SetupTracing(ctx context.Context) (*trace.TracerProvider, error) {
	exporterProtocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	if exporterProtocol == "" {
		exporterProtocol = "http/protobuf"
	}

	exporter, err := createExporter(ctx, exporterProtocol)
	if err != nil {
		return nil, err
	}

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "centrifugo"
	}

	// labels/tags/resources that are common to all traces.
	rs := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		attribute.String("version", build.Version),
	)

	provider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(rs),
	)

	otel.SetTracerProvider(provider)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, // W3C Trace Context format; https://www.w3.org/TR/trace-context/
		),
	)

	otel.SetErrorHandler(&ErrorHandlerImpl{})
	return provider, nil
}

type ErrorHandlerImpl struct{}

func (e ErrorHandlerImpl) Handle(err error) {
	log.Err(err).Msg("opentelemetry error")
}

func createExporter(ctx context.Context, exporterProtocol string) (*otlptrace.Exporter, error) {
	if exporterProtocol == "grpc" {
		return otlptracegrpc.New(
			ctx,
		)
	}

	if exporterProtocol == "http/protobuf" {
		return otlptracehttp.New(
			ctx,
		)
	}

	return nil, fmt.Errorf("unsupported exporter protocol: %s", exporterProtocol)
}
