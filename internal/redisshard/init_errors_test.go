package redisshard

import (
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

// With no address configured the engine default is used.
func TestGetRedisShardConfigs_DefaultAddress(t *testing.T) {
	confs, err := getRedisShardConfigs(configtypes.Redis{})
	require.NoError(t, err)
	require.Len(t, confs, 1)
	require.Equal(t, "127.0.0.1:6379", confs[0].Address)
}

// A TLS configuration which cannot be built is an error to return, not a
// reason to terminate the process.
func TestGetRedisShardConfigs_BadTLS(t *testing.T) {
	bad := configtypes.TLSConfig{Enabled: true, CertPem: "not a certificate", KeyPem: "not a key"}

	conf := configtypes.Redis{Address: []string{"127.0.0.1:6379"}, TLS: bad}
	_, err := getRedisShardConfigs(conf)
	require.Error(t, err)

	conf = configtypes.Redis{
		SentinelAddress:    []string{"127.0.0.1:26379"},
		SentinelMasterName: "mymaster",
		SentinelTLS:        bad,
	}
	_, err = getRedisShardConfigs(conf)
	require.Error(t, err)
}
