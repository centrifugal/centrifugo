package redisshard

import (
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/stretchr/testify/require"
)

func TestGetRedisShardConfigsCommonSettings(t *testing.T) {
	t.Parallel()

	common := configtypes.Redis{
		DB:         2,
		User:       "user",
		Password:   "pass",
		ClientName: "centrifugo-consumer",
		ForceResp2: true,
	}

	standalone := common
	standalone.Address = []string{"redis://127.0.0.1:6379"}

	cluster := common
	cluster.ClusterAddress = []string{"127.0.0.1:7000,127.0.0.1:7001"}

	sentinel := common
	sentinel.SentinelAddress = []string{"127.0.0.1:26379"}
	sentinel.SentinelMasterName = "mymaster"

	for name, redisConf := range map[string]configtypes.Redis{
		"standalone": standalone,
		"cluster":    cluster,
		"sentinel":   sentinel,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			shardConfigs, err := getRedisShardConfigs(redisConf)
			require.NoError(t, err)
			require.Len(t, shardConfigs, 1)
			conf := shardConfigs[0]
			require.Equal(t, 2, conf.DB)
			require.Equal(t, "user", conf.User)
			require.Equal(t, "pass", conf.Password)
			require.Equal(t, "centrifugo-consumer", conf.ClientName)
			require.True(t, conf.ForceRESP2)
		})
	}
}
