package confighelpers

import (
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
)

// CentrifugeRedisMapBroker creates a centrifuge.RedisMapBroker from config types.
func CentrifugeRedisMapBroker(n *centrifuge.Node, prefix string, shards []*centrifuge.RedisShard, cfg configtypes.RedisMapBrokerCommon) (*centrifuge.RedisMapBroker, error) {
	return centrifuge.NewRedisMapBroker(n, centrifuge.RedisMapBrokerConfig{
		Shards:              shards,
		Prefix:              prefix,
		CleanupInterval:     cfg.CleanupInterval.ToDuration(),
		CleanupBatchSize:    cfg.CleanupBatchSize,
		IdempotentResultTTL: cfg.IdempotentResultTTL.ToDuration(),
		SkipPubSub:          cfg.SkipPubSub,
	})
}
