package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegisterKnownEnvVar checks a variable Centrifugo reads directly rather
// than through its configuration stops being reported as unknown once it is
// registered.
//
// Without this an operator who sets one of them - they are documented, so they
// do get set - is told on every start that they set something Centrifugo does
// not know.
func TestRegisterKnownEnvVar(t *testing.T) {
	const name = "CENTRIFUGO_TEST_DIRECTLY_READ_VAR"
	t.Setenv(name, "1")

	require.Contains(t, checkEnvironmentVars(nil), name,
		"a variable which is neither a config key nor registered should be reported")

	RegisterKnownEnvVar(name)
	t.Cleanup(func() { delete(extraKnownEnvVars, name) })

	require.NotContains(t, checkEnvironmentVars(nil), name,
		"a registered variable should not be reported")
}
