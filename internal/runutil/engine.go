package runutil

import (
	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v5/internal/natsbroker"

	"github.com/centrifugal/centrifuge"
)

func memoryEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, string, error) {
	brokerConf, err := memoryBrokerConfig()
	if err != nil {
		return nil, nil, "", err
	}
	broker, err := centrifuge.NewMemoryBroker(n, *brokerConf)
	if err != nil {
		return nil, nil, "", err
	}
	presenceManagerConf, err := memoryPresenceManagerConfig()
	if err != nil {
		return nil, nil, "", err
	}
	presenceManager, err := centrifuge.NewMemoryPresenceManager(n, *presenceManagerConf)
	if err != nil {
		return nil, nil, "", err
	}
	return broker, presenceManager, "", nil
}

func memoryBrokerConfig() (*centrifuge.MemoryBrokerConfig, error) {
	return &centrifuge.MemoryBrokerConfig{}, nil
}

func memoryPresenceManagerConfig() (*centrifuge.MemoryPresenceManagerConfig, error) {
	return &centrifuge.MemoryPresenceManagerConfig{}, nil
}

func NatsBroker(node *centrifuge.Node, cfg config.Config) (*natsbroker.NatsBroker, error) {
	return natsbroker.New(node, cfg.Nats)
}

func redisEngine(n *centrifuge.Node, cfgContainer *config.Container) (*centrifuge.RedisBroker, centrifuge.PresenceManager, string, error) {
	cfg := cfgContainer.Config()
	redisShards, mode, err := confighelpers.CentrifugeRedisShards(n, cfg.Redis.Redis)
	if err != nil {
		return nil, nil, "", err
	}
	broker, err := confighelpers.CentrifugeRedisBroker(
		n, cfg.Redis.Prefix, redisShards, cfg.Redis.RedisBrokerCommon, cfg.Broker == "redisnats")
	if err != nil {
		return nil, nil, mode, err
	}
	presenceManager, err := confighelpers.CentrifugeRedisPresenceManager(
		n, cfg.Redis.Prefix, redisShards, cfg.Redis.RedisPresenceManagerCommon)

	return broker, presenceManager, mode, nil
}
