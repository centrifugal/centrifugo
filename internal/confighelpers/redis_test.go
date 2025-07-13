package confighelpers

import "testing"

func TestMergeModes(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{
			input:    []string{"cluster", "cluster", "standalone", "sentinel", "sentinel", "cluster"},
			expected: "cluster-x2,standalone,sentinel-x2,cluster",
		},
		{
			input:    []string{"standalone"},
			expected: "standalone",
		},
		{
			input:    []string{"sentinel", "sentinel", "sentinel"},
			expected: "sentinel-x3",
		},
		{
			input:    []string{"cluster", "standalone", "sentinel"},
			expected: "cluster,standalone,sentinel",
		},
		{
			input:    []string{},
			expected: "",
		},
		{
			input:    []string{"a", "a", "a", "b", "b", "c", "a"},
			expected: "a-x3,b-x2,c,a",
		},
		{
			input:    []string{"a", "b", "b", "b", "c", "c", "d", "d", "d", "d"},
			expected: "a,b-x3,c-x2,d-x4",
		},
	}

	for _, test := range tests {
		got := mergeModes(test.input)
		if got != test.expected {
			t.Errorf("mergeModes(%v) = %q; want %q", test.input, got, test.expected)
		}
	}
}
