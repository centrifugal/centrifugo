package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetLogURLs tests the RedactedLogURLs function using Redacted method.
func TestGetLogURLs(t *testing.T) {
	t.Run("Single URL with auth info", func(t *testing.T) {
		input := "https://user:password@domain.com/resource"
		expected := []string{"https://user:xxxxx@domain.com/resource"}
		actual := RedactedLogURLs(input)
		require.Equal(t, expected, actual)
	})

	t.Run("Multiple URLs with mixed auth info", func(t *testing.T) {
		input := "https://user:pass@domain.com/resource,https://another.com"
		expected := []string{"https://user:xxxxx@domain.com/resource,https://another.com"}
		actual := RedactedLogURLs(input)
		require.Equal(t, expected, actual)
	})

	t.Run("Multiple URLs with mixed spaces", func(t *testing.T) {
		input := "https://user:pass@domain.com/resource, https://another.com"
		expected := []string{"https://user:xxxxx@domain.com/resource,https://another.com"}
		actual := RedactedLogURLs(input)
		require.Equal(t, expected, actual)
	})

	t.Run("Single URL without auth info", func(t *testing.T) {
		input := "https://domain.com/resource"
		expected := []string{"https://domain.com/resource"}
		actual := RedactedLogURLs(input)
		require.Equal(t, expected, actual)
	})

	t.Run("Invalid URL", func(t *testing.T) {
		input := "://invalid-url"
		expected := []string{"<invalid_url>"}
		actual := RedactedLogURLs(input)
		require.Equal(t, expected, actual)
	})

	t.Run("Mixed valid and invalid URLs", func(t *testing.T) {
		input := "https://user:pass@domain.com/resource, ://invalid-url, https://valid.com"
		expected := []string{"https://user:xxxxx@domain.com/resource,<invalid_url>,https://valid.com"}
		actual := RedactedLogURLs(input)
		require.Equal(t, expected, actual)
	})

	t.Run("Multiple comma-separated URLs with auth", func(t *testing.T) {
		input := "https://user:pass@domain.com, https://admin:admin@another.com, httpss://example.com/resource"
		expected := []string{"https://user:xxxxx@domain.com,https://admin:xxxxx@another.com,httpss://example.com/resource"}
		actual := RedactedLogURLs(input)
		require.Equal(t, expected, actual)
	})

	t.Run("GRPC addresses work correctly", func(t *testing.T) {
		// We use such format for GRPC proxy config.
		input := []string{"grpc://user:pass@127.0.0.1:9000", "grpc://127.0.0.1:10000"}
		expected := []string{"grpc://user:xxxxx@127.0.0.1:9000", "grpc://127.0.0.1:10000"}
		actual := RedactedLogURLs(input...)
		require.Equal(t, expected, actual)
	})
}
