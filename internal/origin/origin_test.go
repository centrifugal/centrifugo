package origin

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatternCheck(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		origin         string
		originPatterns []string
		success        bool
	}{
		{
			name:   "empty_origin",
			origin: "",
			originPatterns: []string{
				"*.example.com",
			},
			success: true,
		},
		{
			name:   "origin_patterns_match",
			origin: "https://two.Example.com",
			originPatterns: []string{
				"*.example.com",
				"foo.com",
			},
			success: true,
		},
		{
			name:   "origin_patterns_no_match",
			origin: "https://two.Example.com",
			originPatterns: []string{
				"foo.com",
				"bar.com",
			},
			success: false,
		},
		{
			name:   "origin_patterns_cyrillic_e_in_origin",
			origin: "https://two.еxample.com",
			originPatterns: []string{
				"*.example.com",
				"foo.com",
			},
			success: false,
		},
		{
			name:   "file_origin",
			origin: "file://",
			originPatterns: []string{
				"file://*",
			},
			success: true,
		},
		{
			name:   "null_origin",
			origin: "null",
			originPatterns: []string{
				"null",
			},
			success: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest("GET", "https://example.com/websocket/connection", nil)
			r.Header.Set("Origin", tc.origin)

			a, err := NewPatternChecker(tc.originPatterns)
			require.NoError(t, err)
			err = a.Check(r)
			if tc.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCheckSameHost(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		origin  string
		url     string
		success bool
	}{
		{
			name:    "empty_origin",
			origin:  "",
			success: true,
			url:     "https://example.com/websocket/connection",
		},
		{
			name:    "invalid_host",
			origin:  "invalid",
			url:     "https://example.com/websocket/connection",
			success: false,
		},
		{
			name:    "unauthorized",
			origin:  "https://example.com",
			url:     "wss://example1.com/websocket/connection",
			success: false,
		},
		{
			name:    "authorized",
			origin:  "https://example.com",
			url:     "wss://example.com/websocket/connection",
			success: true,
		},
		{
			name:    "authorized_case_insensitive",
			origin:  "https://examplE.com",
			url:     "wss://example.com/websocket/connection",
			success: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest("GET", tc.url, nil)
			r.Header.Set("Origin", tc.origin)

			err := CheckSameHost(r)
			if tc.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
