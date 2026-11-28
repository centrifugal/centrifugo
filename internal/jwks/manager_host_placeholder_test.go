package jwks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewManagerAcceptsHostPlaceholders(t *testing.T) {
	for template, want := range map[string]string{
		"https://{{tenant}}.example.com/.well-known/jwks.json": "https://acme.example.com/.well-known/jwks.json",
		"https://{{tenant}}/protocol/openid-connect/certs":     "https://acme/protocol/openid-connect/certs",
	} {
		t.Run(template, func(t *testing.T) {
			manager, err := NewManager(template)
			require.NoError(t, err)
			defer func() { _ = manager.Close() }()

			got, err := manager.resolveURL(map[string]any{"tenant": "acme"})
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}
