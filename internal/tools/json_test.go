package tools

import (
	"testing"
)

func TestIsValidJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "empty data",
			input:    []byte{},
			expected: false,
		},
		{
			name:     "valid json object",
			input:    []byte(`{"key": "value"}`),
			expected: true,
		},
		{
			name:     "valid json array",
			input:    []byte(`[1, 2, 3]`),
			expected: true,
		},
		{
			name:     "valid json string",
			input:    []byte(`"hello"`),
			expected: true,
		},
		{
			name:     "valid json number",
			input:    []byte(`123`),
			expected: true,
		},
		{
			name:     "valid json boolean true",
			input:    []byte(`true`),
			expected: true,
		},
		{
			name:     "valid json boolean false",
			input:    []byte(`false`),
			expected: true,
		},
		{
			name:     "valid json null",
			input:    []byte(`null`),
			expected: true,
		},
		{
			name:     "valid empty json object",
			input:    []byte(`{}`),
			expected: true,
		},
		{
			name:     "valid empty json array",
			input:    []byte(`[]`),
			expected: true,
		},
		{
			name:     "invalid json - missing closing brace",
			input:    []byte(`{"key": "value"`),
			expected: false,
		},
		{
			name:     "invalid json - plain text",
			input:    []byte(`hello world`),
			expected: false,
		},
		{
			name:     "invalid json - trailing comma",
			input:    []byte(`{"key": "value",}`),
			expected: false,
		},
		{
			name:     "invalid json - single quote",
			input:    []byte(`{'key': 'value'}`),
			expected: false,
		},
		{
			name:     "invalid json - unquoted key",
			input:    []byte(`{key: "value"}`),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidJSON(tt.input)
			if got != tt.expected {
				t.Errorf("IsValidJSON() = %v, want %v", got, tt.expected)
			}
		})
	}
}
