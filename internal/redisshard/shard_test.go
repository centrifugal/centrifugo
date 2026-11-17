package redisshard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Sentinel connections are dialled with the data connections' connect timeout
// and keep-alive, whether the shard is configured with SentinelAddresses or a
// redis+sentinel:// address (centrifugo#1325).
func TestRedisClientOptions_SentinelDialer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		conf RedisShardConfig
	}{
		{
			name: "sentinel addresses",
			conf: RedisShardConfig{
				SentinelAddresses:  []string{"127.0.0.1:26379"},
				SentinelMasterName: "mymaster",
			},
		},
		{
			name: "sentinel URL",
			conf: RedisShardConfig{
				Address: "redis+sentinel://127.0.0.1:26379?sentinel_master_name=mymaster&connect_timeout=3s",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			conf := tc.conf
			conf.ConnectTimeout = defaultRedisConnectTimeout
			conf.IOTimeout = defaultRedisIOTimeout
			options, _, isSentinel, _, err := redisClientOptions(conf)
			require.NoError(t, err)
			require.True(t, isSentinel)
			require.NotZero(t, options.Dialer.Timeout)
			require.NotZero(t, options.Dialer.KeepAlive)
			require.Equal(t, options.Dialer.Timeout, options.Sentinel.Dialer.Timeout)
			require.Equal(t, options.Dialer.KeepAlive, options.Sentinel.Dialer.KeepAlive)
		})
	}
	// The address's connect_timeout reaches the Sentinel connections too.
	options, _, _, _, err := redisClientOptions(RedisShardConfig{
		Address:        "redis+sentinel://127.0.0.1:26379?sentinel_master_name=mymaster&connect_timeout=3s",
		ConnectTimeout: defaultRedisConnectTimeout,
		IOTimeout:      defaultRedisIOTimeout,
	})
	require.NoError(t, err)
	require.Equal(t, 3*time.Second, options.Sentinel.Dialer.Timeout)
}
