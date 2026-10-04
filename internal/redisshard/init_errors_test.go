package redisshard

import (
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

// An explicitly empty address is an error, not a fallback to localhost.
func TestGetRedisShardConfigs_NoAddress(t *testing.T) {
	_, err := getRedisShardConfigs(configtypes.Redis{Password: "secret"})
	require.Error(t, err)
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
