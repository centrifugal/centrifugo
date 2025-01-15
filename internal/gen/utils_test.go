package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CamelCaseString", "camel_case_string"},
		{"TestID123", "test_id123"},
		{"NoChange", "no_change"},
		{"RPC", "rpc"},
		{"", ""},
		{"already_snake_case", "already_snake_case"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CamelToSnake(tt.input)
			require.Equal(t, tt.expected, result, "Unexpected result for input: %s", tt.input)
		})
	}
}
