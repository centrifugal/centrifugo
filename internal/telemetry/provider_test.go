package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

func resourceAttr(t *testing.T, attrs []attribute.KeyValue, key attribute.Key) (string, bool) {
	t.Helper()
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.AsString(), true
		}
	}
	return "", false
}

func TestNewResourceDefaults(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")

	rs, err := newResource(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := rs.Attributes()

	if v, _ := resourceAttr(t, attrs, semconv.ServiceNameKey); v != "centrifugo" {
		t.Fatalf("unexpected service.name: %q", v)
	}
	if v, _ := resourceAttr(t, attrs, semconv.ServiceInstanceIDKey); v != "node-1" {
		t.Fatalf("unexpected service.instance.id: %q", v)
	}
}

func TestNewResourceEnvOverridesAndMerges(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "gcp.project_id=my-project,service.instance.id=custom-id")
	t.Setenv("OTEL_SERVICE_NAME", "my-centrifugo")

	rs, err := newResource(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := rs.Attributes()

	if v, _ := resourceAttr(t, attrs, semconv.ServiceNameKey); v != "my-centrifugo" {
		t.Fatalf("unexpected service.name: %q", v)
	}
	if v, _ := resourceAttr(t, attrs, semconv.ServiceInstanceIDKey); v != "custom-id" {
		t.Fatalf("env must override default service.instance.id, got: %q", v)
	}
	if v, ok := resourceAttr(t, attrs, "gcp.project_id"); !ok || v != "my-project" {
		t.Fatalf("OTEL_RESOURCE_ATTRIBUTES not merged into resource, gcp.project_id: %q (present: %v)", v, ok)
	}
	if v, ok := resourceAttr(t, attrs, "version"); !ok || v == "" {
		t.Fatalf("version attribute missing: %q (present: %v)", v, ok)
	}
}

func TestNewResourceMalformedEnv(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "missing-equals-sign")
	t.Setenv("OTEL_SERVICE_NAME", "")

	if _, err := newResource(context.Background(), "node-1"); err == nil {
		t.Fatal("expected error for malformed OTEL_RESOURCE_ATTRIBUTES")
	}
}

type errorDetector struct{}

func (errorDetector) Detect(context.Context) (*resource.Resource, error) {
	return nil, context.DeadlineExceeded
}

func TestNewResourceDetectorError(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")

	if _, err := newResource(context.Background(), "node-1", errorDetector{}); err == nil {
		t.Fatal("expected error when detector fails")
	}
}

type staticDetector []attribute.KeyValue

func (d staticDetector) Detect(context.Context) (*resource.Resource, error) {
	return resource.NewSchemaless(d...), nil
}

func TestNewResourceDetectorPrecedence(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "cloud.region=env-region")
	t.Setenv("OTEL_SERVICE_NAME", "")

	det := staticDetector{
		attribute.String("cloud.region", "detected-region"),
		attribute.String("k8s.cluster.name", "detected-cluster"),
		semconv.ServiceNameKey.String("detected-name"),
	}
	rs, err := newResource(context.Background(), "node-1", det)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs := rs.Attributes()

	if v, _ := resourceAttr(t, attrs, "cloud.region"); v != "env-region" {
		t.Fatalf("env must override detector, cloud.region: %q", v)
	}
	if v, _ := resourceAttr(t, attrs, "k8s.cluster.name"); v != "detected-cluster" {
		t.Fatalf("detector attribute lost, k8s.cluster.name: %q", v)
	}
	if v, _ := resourceAttr(t, attrs, semconv.ServiceNameKey); v != "centrifugo" {
		t.Fatalf("Centrifugo defaults must override detector, service.name: %q", v)
	}
	if v, _ := resourceAttr(t, attrs, semconv.ServiceInstanceIDKey); v != "node-1" {
		t.Fatalf("unexpected service.instance.id: %q", v)
	}
}
