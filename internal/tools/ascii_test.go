package tools

import "testing"

func Test_IsASCII(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: true,
		},
		{
			name:     "ascii string",
			input:    "hello",
			expected: true,
		},
		{
			name:     "non-ascii string",
			input:    "こんにちは",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsASCII(tt.input) != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, !tt.expected)
			}
		})
	}
}
