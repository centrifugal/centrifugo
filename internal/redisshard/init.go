package redisshard

import (
	"fmt"
	"net"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/rs/zerolog/log"
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

func addRedisShardCommonSettings(shardConf *RedisShardConfig, redisConf configtypes.Redis) {
	shardConf.DB = redisConf.DB
	shardConf.User = redisConf.User
	shardConf.Password = redisConf.Password
	if redisConf.TLS.Enabled {
		tlsConfig, err := redisConf.TLS.ToGoTLSConfig("redis")
		if err != nil {
			log.Fatal().Err(err).Msg("error creating Redis TLS config")
		}
		shardConf.TLSConfig = tlsConfig
	}
	shardConf.ConnectTimeout = redisConf.ConnectTimeout.ToDuration()
	shardConf.IOTimeout = redisConf.IOTimeout.ToDuration()
	shardConf.ForceRESP2 = redisConf.ForceResp2
	shardConf.ReplicaClientEnabled = redisConf.ReplicaClient.Enabled
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
			addRedisShardCommonSettings(conf, redisConf)
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
			addRedisShardCommonSettings(conf, redisConf)
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
					log.Fatal().Err(err).Msg("error creating Redis Sentinel TLS config")
				}
				conf.SentinelTLSConfig = tlsConfig
			}
			shardConfigs = append(shardConfigs, *conf)
		}
		return shardConfigs, nil
	}

	redisAddresses := redisConf.Address
	if len(redisAddresses) == 0 {
		return nil, fmt.Errorf("no Redis address configured")
	}
	for _, redisAddress := range redisAddresses {
		conf := &RedisShardConfig{
			Address: redisAddress,
		}
		addRedisShardCommonSettings(conf, redisConf)
		shardConfigs = append(shardConfigs, *conf)
	}
	return shardConfigs, nil
}
