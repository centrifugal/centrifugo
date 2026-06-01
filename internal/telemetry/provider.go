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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

// googleCloudAuthScope is the OAuth2 scope used when authenticating to Google
// Cloud's OTLP endpoint (telemetry.googleapis.com) with Application Default
// Credentials.
const googleCloudAuthScope = "https://www.googleapis.com/auth/cloud-platform"

func SetupTracing(ctx context.Context, googleCloudAuth bool) (*trace.TracerProvider, error) {
	exporterProtocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	if exporterProtocol == "" {
		exporterProtocol = "http/protobuf"
	}

	exporter, err := createExporter(ctx, exporterProtocol, googleCloudAuth)
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

func createExporter(ctx context.Context, exporterProtocol string, googleCloudAuth bool) (*otlptrace.Exporter, error) {
	if exporterProtocol == "grpc" {
		var opts []otlptracegrpc.Option
		if googleCloudAuth {
			// Inject Google Cloud Application Default Credentials as per-RPC
			// OAuth2 tokens so the exporter can authenticate against Google
			// Cloud's OTLP endpoint (telemetry.googleapis.com). The token
			// source refreshes automatically; the metadata server lookup is
			// lazy and happens on first export, not at startup.
			creds, err := oauth.NewApplicationDefault(ctx, googleCloudAuthScope)
			if err != nil {
				return nil, fmt.Errorf("error creating Google Cloud application default credentials: %w", err)
			}
			opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)))
		}
		return otlptracegrpc.New(ctx, opts...)
	}

	if exporterProtocol == "http/protobuf" {
		if googleCloudAuth {
			return nil, fmt.Errorf("opentelemetry google_cloud_auth requires the grpc exporter protocol (OTEL_EXPORTER_OTLP_PROTOCOL=grpc)")
		}
		return otlptracehttp.New(
			ctx,
		)
	}

	return nil, fmt.Errorf("unsupported exporter protocol: %s", exporterProtocol)
}
