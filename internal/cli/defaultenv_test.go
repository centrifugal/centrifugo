package cli

import (
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/stretchr/testify/require"
)

// Options tagged expose:"-" are known (not reported as unknown when set) but
// not listed by defaultenv.
func TestDefaultEnvHiddenVars(t *testing.T) {
	_, meta, err := config.GetConfig(nil, "")
	require.NoError(t, err)
	info, ok := meta.KnownEnvVars["CENTRIFUGO_BROKER_REDISNATS_NATS_PREFIX"]
	require.True(t, ok, "hidden env var is still known")
	require.True(t, info.Hidden)
	require.False(t, meta.KnownEnvVars["CENTRIFUGO_BROKER_TYPE"].Hidden)
}
