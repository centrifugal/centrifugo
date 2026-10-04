package redisshard

import (
	"fmt"
	"net"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

func BuildRedisShards(redisConf configtypes.Redis) ([]*RedisShard, error) {
	redisShardConfigs, err := getRedisShardConfigs(redisConf)
	if err != nil {
		return nil, err
	}
	redisShards := make([]*RedisShard, 0, len(redisShardConfigs))

	for _, redisCfg := range redisShardConfigs {
		redisShard, err := NewRedisShard(redisCfg)
		if err != nil {
			return nil, err
		}
		redisShards = append(redisShards, redisShard)
	}
	return redisShards, nil
}

func addRedisShardCommonSettings(shardConf *RedisShardConfig, redisConf configtypes.Redis) error {
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
	shardConf.ReplicaClientEnabled = redisConf.ReplicaClient.Enabled
	return nil
}

func getRedisShardConfigs(redisConf configtypes.Redis) ([]RedisShardConfig, error) {
	var shardConfigs []RedisShardConfig

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
			conf := &RedisShardConfig{
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
			conf := &RedisShardConfig{
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
				tlsConfig, err := redisConf.SentinelTLS.ToGoTLSConfig("redis_sentinel")
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
		// The address has a default, so it is empty only when set so (an empty
		// env var, say). Refuse rather than send the credentials to localhost.
		return nil, fmt.Errorf("no Redis address configured")
	}
	for _, redisAddress := range redisAddresses {
		conf := &RedisShardConfig{
			Address: redisAddress,
		}
		if err := addRedisShardCommonSettings(conf, redisConf); err != nil {
			return nil, err
		}
		shardConfigs = append(shardConfigs, *conf)
	}
	return shardConfigs, nil
}
