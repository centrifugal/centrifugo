package telemetry

import (
	"context"
	"fmt"
	"net/http"
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
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

// googleCloudAuthScope is the OAuth2 scope used when authenticating to Google
// Cloud's OTLP endpoint (telemetry.googleapis.com) with Application Default
// Credentials.
const googleCloudAuthScope = "https://www.googleapis.com/auth/cloud-platform"

func SetupTracing(ctx context.Context, googleCloudADCAuth bool) (*trace.TracerProvider, error) {
	exporterProtocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	if exporterProtocol == "" {
		exporterProtocol = "http/protobuf"
	}

	exporter, err := createExporter(ctx, exporterProtocol, googleCloudADCAuth)
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

func createExporter(ctx context.Context, exporterProtocol string, googleCloudADCAuth bool) (*otlptrace.Exporter, error) {
	if exporterProtocol == "grpc" {
		var opts []otlptracegrpc.Option
		if googleCloudADCAuth {
			// Inject Google Cloud Application Default Credentials as per-RPC
			// OAuth2 tokens so the exporter can authenticate against Google
			// Cloud's OTLP endpoint (telemetry.googleapis.com). The access
			// token is minted lazily on first export and then cached and
			// auto-refreshed. Note: resolving ADC here (at startup) may do a
			// one-time metadata-server probe when running on GCE without an
			// explicit credentials file.
			creds, err := oauth.NewApplicationDefault(ctx, googleCloudAuthScope)
			if err != nil {
				return nil, fmt.Errorf("error creating Google Cloud application default credentials: %w", err)
			}
			opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)))
		}
		return otlptracegrpc.New(ctx, opts...)
	}

	if exporterProtocol == "http/protobuf" {
		var opts []otlptracehttp.Option
		if googleCloudADCAuth {
			// Same as the gRPC path, but ADC tokens are attached by an OAuth2
			// http.Client transport (which also refreshes them automatically)
			// since the HTTP exporter has no per-RPC credentials concept.
			client, err := googleCloudADCHTTPClient(ctx)
			if err != nil {
				return nil, err
			}
			opts = append(opts, otlptracehttp.WithHTTPClient(client))
		}
		return otlptracehttp.New(ctx, opts...)
	}

	return nil, fmt.Errorf("unsupported exporter protocol: %s", exporterProtocol)
}

// googleCloudADCHTTPClient returns an *http.Client that authenticates outgoing
// requests with Google Cloud Application Default Credentials. The OAuth2
// transport mints the access token lazily on first request and then caches and
// auto-refreshes it. Resolving ADC (here, at startup) may perform a one-time
// metadata-server probe when running on GCE without an explicit credentials file.
func googleCloudADCHTTPClient(ctx context.Context) (*http.Client, error) {
	ts, err := google.DefaultTokenSource(ctx, googleCloudAuthScope)
	if err != nil {
		return nil, fmt.Errorf("error creating Google Cloud application default credentials: %w", err)
	}
	return oauth2.NewClient(ctx, ts), nil
}
