package confighelpers

import (
	"fmt"
	"net"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
)

func CentrifugeRedisBroker(n *centrifuge.Node, prefix string, shards []*centrifuge.RedisShard, cfg configtypes.RedisBrokerCommon, skipPubSub bool) (*centrifuge.RedisBroker, error) {
	return centrifuge.NewRedisBroker(n, centrifuge.RedisBrokerConfig{
		Shards:     shards,
		Prefix:     prefix,
		UseLists:   cfg.UseLists,
		SkipPubSub: skipPubSub,
	})
}

func CentrifugeRedisPresenceManager(n *centrifuge.Node, prefix string, shards []*centrifuge.RedisShard, cfg configtypes.RedisPresenceManagerCommon) (*centrifuge.RedisPresenceManager, error) {
	presenceManagerConfig := centrifuge.RedisPresenceManagerConfig{
		Shards:          shards,
		Prefix:          prefix,
		PresenceTTL:     cfg.PresenceTTL.ToDuration(),
		UseHashFieldTTL: cfg.PresenceHashFieldTTL,
	}
	if cfg.PresenceUserMapping {
		presenceManagerConfig.EnableUserMapping = func(_ string) bool {
			return true
		}
	}
	return centrifuge.NewRedisPresenceManager(n, presenceManagerConfig)
}

func addRedisShardCommonSettings(shardConf *centrifuge.RedisShardConfig, redisConf configtypes.Redis) error {
	shardConf.DB = redisConf.DB
	shardConf.User = redisConf.User
	shardConf.Password = redisConf.Password
	shardConf.ClientName = redisConf.ClientName

	if redisConf.TLS.Enabled {
		tlsConfig, err := redisConf.TLS.ToGoTLSConfig("redis")
		if err != nil {
			return fmt.Errorf("error creating Redis TLS config: %v", err)
		}
		shardConf.TLSConfig = tlsConfig
	}
	shardConf.ConnectTimeout = redisConf.ConnectTimeout.ToDuration()
	shardConf.IOTimeout = redisConf.IOTimeout.ToDuration()
	shardConf.ForceRESP2 = redisConf.ForceResp2
	return nil
}

func getRedisShardConfigs(redisConf configtypes.Redis) ([]centrifuge.RedisShardConfig, error) {
	var shardConfigs []centrifuge.RedisShardConfig

	clusterShards := redisConf.ClusterAddress
	var useCluster bool
	if len(clusterShards) > 0 {
		useCluster = true
	}

	if useCluster {
		for _, clusterAddress := range clusterShards {
			clusterAddresses := strings.Split(clusterAddress, ",")
			for _, address := range clusterAddresses {
				if _, _, err := net.SplitHostPort(address); err != nil {
					return nil, fmt.Errorf("malformed Redis Cluster address: %s", address)
				}
			}
			conf := &centrifuge.RedisShardConfig{
				ClusterAddresses: clusterAddresses,
			}
			if err := addRedisShardCommonSettings(conf, redisConf); err != nil {
				return nil, err
			}
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, nil
	}

	sentinelShards := redisConf.SentinelAddress
	var useSentinel bool
	if len(sentinelShards) > 0 {
		useSentinel = true
	}

	if useSentinel {
		for _, sentinelAddress := range sentinelShards {
			sentinelAddresses := strings.Split(sentinelAddress, ",")
			for _, address := range sentinelAddresses {
				if _, _, err := net.SplitHostPort(address); err != nil {
					return nil, fmt.Errorf("malformed Redis Sentinel address: %s", address)
				}
			}
			conf := &centrifuge.RedisShardConfig{
				SentinelAddresses: sentinelAddresses,
			}
			if err := addRedisShardCommonSettings(conf, redisConf); err != nil {
				return nil, err
			}
			conf.SentinelUser = redisConf.SentinelUser
			conf.SentinelPassword = redisConf.SentinelPassword
			conf.SentinelMasterName = redisConf.SentinelMasterName
			if conf.SentinelMasterName == "" {
				return nil, fmt.Errorf("master name must be set when using Redis Sentinel")
			}
			conf.SentinelClientName = redisConf.SentinelClientName
			if redisConf.SentinelTLS.Enabled {
				tlsConfig, err := redisConf.TLS.ToGoTLSConfig("redis_sentinel")
				if err != nil {
					return nil, fmt.Errorf("error creating Redis Sentinel TLS config: %v", err)
				}
				conf.SentinelTLSConfig = tlsConfig
			}
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, nil
	}

	redisAddresses := redisConf.Address
	if len(redisAddresses) == 0 {
		redisAddresses = []string{"127.0.0.1:6379"}
	}
	for _, redisAddress := range redisAddresses {
		conf := &centrifuge.RedisShardConfig{
			Address: redisAddress,
		}
		if err := addRedisShardCommonSettings(conf, redisConf); err != nil {
			return nil, err
		}
		shardConfigs = append(shardConfigs, *conf)
	}

	return shardConfigs, nil
}

func CentrifugeRedisShards(n *centrifuge.Node, redisConf configtypes.Redis) ([]*centrifuge.RedisShard, string, error) {
	redisShardConfigs, err := getRedisShardConfigs(redisConf)
	if err != nil {
		return nil, "", err
	}
	redisShards := make([]*centrifuge.RedisShard, 0, len(redisShardConfigs))

	modes := make([]string, 0, len(redisShardConfigs))

	for _, shardConf := range redisShardConfigs {
		redisShard, err := centrifuge.NewRedisShard(n, shardConf)
		if err != nil {
			return nil, "", err
		}
		modes = append(modes, string(redisShard.Mode()))
		redisShards = append(redisShards, redisShard)
	}

	mode := mergeModes(modes)
	if len(redisShards) > 1 {
		mode = fmt.Sprintf("sharded(%d):%s", len(modes), mode)
	}

	return redisShards, mode, nil
}

// [cluster,cluster,standalone,sentinel,sentinel] => "cluster-x2,standalone,sentinel-x2".
func mergeModes(modes []string) string {
	if len(modes) == 0 {
		return ""
	}

	var result []string
	count := 1

	for i := 1; i < len(modes); i++ {
		if modes[i] == modes[i-1] {
			count++
		} else {
			if count > 1 {
				result = append(result, fmt.Sprintf("%s-x%d", modes[i-1], count))
			} else {
				result = append(result, modes[i-1])
			}
			count = 1
		}
	}

	if count > 1 {
		result = append(result, fmt.Sprintf("%s-x%d", modes[len(modes)-1], count))
	} else {
		result = append(result, modes[len(modes)-1])
	}

	return strings.Join(result, ",")
}
