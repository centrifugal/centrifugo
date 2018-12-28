package graphite

import (
	"testing"
)

func TestPreparePathComponent(t *testing.T) {
	testCases := []struct {
		in, out string
	}{
		{in: "service", out: "service"},
		{in: "service.local", out: "service_local"},
		{in: "service.prod.", out: "service_prod_"},
		{in: "приvет", out: "___v__"},
	}

	for _, tc := range testCases {
		if want, got := tc.out, PreparePathComponent(tc.in); want != got {
			t.Fatalf("error, got sanitized string %s, want %s", got, want)
		}
	}
}
