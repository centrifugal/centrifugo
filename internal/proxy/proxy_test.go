package proxy

import (
	"context"
	"net/http"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/middleware"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// TestRequestHeaders_StaticHeadersOverride tests that static headers are set
// but can be overridden by client headers only when the header key is explicitly
// allowed in the configuration.
func TestRequestHeaders_StaticHeadersOverride(t *testing.T) {
	tests := []struct {
		name           string
		staticHeaders  map[string]string
		clientHeaders  http.Header
		allowedHeaders []string
		expectedResult map[string]string
	}{
		{
			name: "static header not overridden when key not allowed",
			staticHeaders: map[string]string{
				"X-Static-Header": "static-value",
			},
			clientHeaders: http.Header{
				"X-Static-Header": []string{"client-value"},
			},
			allowedHeaders: []string{}, // No headers allowed
			expectedResult: map[string]string{
				"X-Static-Header": "static-value",
				"Content-Type":    "application/json",
			},
		},
		{
			name: "static header overridden when key is allowed",
			staticHeaders: map[string]string{
				"X-Static-Header": "static-value",
			},
			clientHeaders: http.Header{
				"X-Static-Header": []string{"client-value"},
			},
			allowedHeaders: []string{"x-static-header"}, // Header is allowed
			expectedResult: map[string]string{
				"X-Static-Header": "client-value",
				"Content-Type":    "application/json",
			},
		},
		{
			name: "multiple static headers with partial override",
			staticHeaders: map[string]string{
				"X-Static-1": "static-1",
				"X-Static-2": "static-2",
			},
			clientHeaders: http.Header{
				"X-Static-1": []string{"client-1"},
				"X-Static-2": []string{"client-2"},
			},
			allowedHeaders: []string{"x-static-1"}, // Only first header is allowed
			expectedResult: map[string]string{
				"X-Static-1":   "client-1", // Overridden (allowed)
				"X-Static-2":   "static-2", // Not overridden (not allowed)
				"Content-Type": "application/json",
			},
		},
		{
			name: "client header not in static set but allowed",
			staticHeaders: map[string]string{
				"X-Static-Header": "static-value",
			},
			clientHeaders: http.Header{
				"X-Client-Only": []string{"client-only-value"},
			},
			allowedHeaders: []string{"x-client-only"},
			expectedResult: map[string]string{
				"X-Static-Header": "static-value",
				"X-Client-Only":   "client-only-value",
				"Content-Type":    "application/json",
			},
		},
		{
			name:          "no static headers",
			staticHeaders: map[string]string{},
			clientHeaders: http.Header{
				"X-Client-Header": []string{"client-value"},
			},
			allowedHeaders: []string{"x-client-header"},
			expectedResult: map[string]string{
				"X-Client-Header": "client-value",
				"Content-Type":    "application/json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = middleware.SetHeadersToContext(ctx, tt.clientHeaders)

			result := requestHeaders(ctx, tt.allowedHeaders, []string{}, tt.staticHeaders)

			require.Equal(t, len(tt.expectedResult), len(result), "number of headers should match")
			for expectedKey, expectedValue := range tt.expectedResult {
				require.Equal(t, expectedValue, result.Get(expectedKey),
					"header %s should have value %s", expectedKey, expectedValue)
			}
		})
	}
}

// TestRequestMetadata_StaticMetadataOverride tests that static metadata is set
// but can be overridden by client metadata only when the metadata key is explicitly
// allowed in the configuration.
func TestRequestMetadata_StaticMetadataOverride(t *testing.T) {
	tests := []struct {
		name            string
		staticMetadata  map[string]string
		clientMetadata  metadata.MD
		allowedMetaKeys []string
		expectedResult  map[string]string
	}{
		{
			name: "static metadata not overridden when key not allowed",
			staticMetadata: map[string]string{
				"x-static-meta": "static-value",
			},
			clientMetadata: metadata.MD{
				"x-static-meta": []string{"client-value"},
			},
			allowedMetaKeys: []string{}, // No metadata keys allowed
			expectedResult: map[string]string{
				"x-static-meta": "static-value",
			},
		},
		{
			name: "static metadata overridden when key is allowed",
			staticMetadata: map[string]string{
				"x-static-meta": "static-value",
			},
			clientMetadata: metadata.MD{
				"x-static-meta": []string{"client-value"},
			},
			allowedMetaKeys: []string{"x-static-meta"}, // Metadata key is allowed
			expectedResult: map[string]string{
				"x-static-meta": "client-value",
			},
		},
		{
			name: "multiple static metadata with partial override",
			staticMetadata: map[string]string{
				"x-static-1": "static-1",
				"x-static-2": "static-2",
			},
			clientMetadata: metadata.MD{
				"x-static-1": []string{"client-1"},
				"x-static-2": []string{"client-2"},
			},
			allowedMetaKeys: []string{"x-static-1"}, // Only first key is allowed
			expectedResult: map[string]string{
				"x-static-1": "client-1", // Overridden (allowed)
				"x-static-2": "static-2", // Not overridden (not allowed)
			},
		},
		{
			name: "client metadata not in static set but allowed",
			staticMetadata: map[string]string{
				"x-static-meta": "static-value",
			},
			clientMetadata: metadata.MD{
				"x-client-only": []string{"client-only-value"},
			},
			allowedMetaKeys: []string{"x-client-only"},
			expectedResult: map[string]string{
				"x-static-meta": "static-value",
				"x-client-only": "client-only-value",
			},
		},
		{
			name:           "no static metadata",
			staticMetadata: map[string]string{},
			clientMetadata: metadata.MD{
				"x-client-meta": []string{"client-value"},
			},
			allowedMetaKeys: []string{"x-client-meta"},
			expectedResult: map[string]string{
				"x-client-meta": "client-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = metadata.NewIncomingContext(ctx, tt.clientMetadata)

			result := requestMetadata(ctx, []string{}, tt.allowedMetaKeys, tt.staticMetadata)

			require.Equal(t, len(tt.expectedResult), len(result), "number of metadata entries should match")
			for expectedKey, expectedValue := range tt.expectedResult {
				values := result.Get(expectedKey)
				require.Len(t, values, 1, "metadata key %s should have exactly one value", expectedKey)
				require.Equal(t, expectedValue, values[0],
					"metadata %s should have value %s", expectedKey, expectedValue)
			}
		})
	}
}
