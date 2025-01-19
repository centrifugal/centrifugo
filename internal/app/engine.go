package app

import (
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/natsbroker"
	"github.com/centrifugal/centrifugo/v6/internal/redisnatsbroker"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
)

type engineModes struct {
	engineMode          string
	brokerMode          string
	presenceManagerMode string
}

func configureEngines(node *centrifuge.Node, cfgContainer *config.Container) (engineModes, error) {
	cfg := cfgContainer.Config()

	var modes engineModes

	var broker centrifuge.Broker
	var presenceManager centrifuge.PresenceManager

	if !cfg.Broker.Enabled || !cfg.PresenceManager.Enabled {
		var err error
		switch cfg.Engine.Type {
		case "memory":
			broker, presenceManager, err = createMemoryEngine(node)
		case "redis":
			broker, presenceManager, modes.engineMode, err = createRedisEngine(node, cfgContainer)
		default:
			return modes, fmt.Errorf("unknown engine type: %s", cfg.Engine.Type)
		}
		event := log.Info().Str("engine_type", cfg.Engine.Type)
		if modes.engineMode != "" {
			event.Str("engine_mode", modes.engineMode)
		}
		event.Msg("initializing redis engine")
		if err != nil {
			return modes, fmt.Errorf("error creating redis engine: %v", err)
		}
	} else {
		log.Info().Msgf("both broker and presence manager enabled, skip engine initialization")
	}

	if cfg.Broker.Enabled {
		var err error
		switch cfg.Broker.Type {
		case "memory":
			broker, err = createMemoryBroker(node)
		case "redis":
			broker, modes.brokerMode, err = createRedisBroker(node, cfgContainer)
		case "nats":
			broker, err = NatsBroker(node, cfg)
			modes.brokerMode = "nats"
		case "redisnats":
			if !cfg.EnableUnreleasedFeatures {
				return modes, fmt.Errorf("redisnats broker requires enable_unreleased_features on")
			}
			log.Warn().Msg("redisnats broker is not released, it may be changed or removed at any point")
			redisBroker, redisBrokerMode, err := createRedisBroker(node, cfgContainer)
			if err != nil {
				return modes, fmt.Errorf("error creating redis broker: %v", err)
			}
			modes.brokerMode = redisBrokerMode + "_nats"
			natsBroker, err := NatsBroker(node, cfg)
			if err != nil {
				return modes, fmt.Errorf("error creating nats broker: %v", err)
			}
			broker, err = redisnatsbroker.New(natsBroker, redisBroker)
			if err != nil {
				return modes, fmt.Errorf("error creating redisnats broker: %v", err)
			}
		default:
			return modes, fmt.Errorf("unknown broker type: %s", cfg.Broker.Type)
		}
		if err != nil {
			return modes, fmt.Errorf("error creating broker: %v", err)
		}
		event := log.Info().Str("broker_type", cfg.Broker.Type)
		if modes.brokerMode != "" {
			event.Str("broker_mode", modes.brokerMode)
		}
		event.Msg("broker is enabled, using it instead of broker from engine")
	} else {
		log.Info().Msgf("explicit broker not provided, using the one from engine")
	}

	if cfg.PresenceManager.Enabled {
		var err error
		switch cfg.PresenceManager.Type {
		case "memory":
			presenceManager, err = createMemoryPresenceManager(node)
		case "redis":
			presenceManager, modes.presenceManagerMode, err = createRedisPresenceManager(node, cfgContainer)
		default:
			return modes, fmt.Errorf("unknown presence manager type: %s", cfg.PresenceManager.Type)
		}
		if err != nil {
			return modes, fmt.Errorf("error creating presence manager: %v", err)
		}
		event := log.Info().Str("presence_manager_type", cfg.PresenceManager.Type)
		if modes.presenceManagerMode != "" {
			event.Str("presence_manager_mode", modes.presenceManagerMode)
		}
		event.Msg("presence manager is enabled, using it instead of presence manager from engine")
	} else {
		log.Info().Msgf("explicit presence manager not provided, using the one from engine")
	}

	node.SetBroker(broker)
	node.SetPresenceManager(presenceManager)
	return modes, nil
}

func createMemoryBroker(n *centrifuge.Node) (centrifuge.Broker, error) {
	brokerConf, err := memoryBrokerConfig()
	if err != nil {
		return nil, err
	}
	broker, err := centrifuge.NewMemoryBroker(n, *brokerConf)
	if err != nil {
		return nil, err
	}
	return broker, nil
}

func createMemoryPresenceManager(n *centrifuge.Node) (centrifuge.PresenceManager, error) {
	presenceManagerConf, err := memoryPresenceManagerConfig()
	if err != nil {
		return nil, err
	}
	return centrifuge.NewMemoryPresenceManager(n, *presenceManagerConf)
}

func createMemoryEngine(n *centrifuge.Node) (centrifuge.Broker, centrifuge.PresenceManager, error) {
	broker, err := createMemoryBroker(n)
	if err != nil {
		return nil, nil, err
	}
	presenceManager, err := createMemoryPresenceManager(n)
	if err != nil {
		return nil, nil, err
	}
	return broker, presenceManager, nil
}

func memoryBrokerConfig() (*centrifuge.MemoryBrokerConfig, error) {
	return &centrifuge.MemoryBrokerConfig{}, nil
}

func memoryPresenceManagerConfig() (*centrifuge.MemoryPresenceManagerConfig, error) {
	return &centrifuge.MemoryPresenceManagerConfig{}, nil
}

func NatsBroker(node *centrifuge.Node, cfg config.Config) (*natsbroker.NatsBroker, error) {
	return natsbroker.New(node, cfg.Broker.Nats)
}

func createRedisEngine(n *centrifuge.Node, cfgContainer *config.Container) (*centrifuge.RedisBroker, centrifuge.PresenceManager, string, error) {
	cfg := cfgContainer.Config()
	redisShards, mode, err := confighelpers.CentrifugeRedisShards(n, cfg.Engine.Redis.Redis)
	if err != nil {
		return nil, nil, mode, fmt.Errorf("error creating Redis shards: %w", err)
	}

	var broker *centrifuge.RedisBroker
	if !cfg.Broker.Enabled {
		broker, err = confighelpers.CentrifugeRedisBroker(
			n, cfg.Engine.Redis.Prefix, redisShards, cfg.Engine.Redis.RedisBrokerCommon, false)
		if err != nil {
			return nil, nil, mode, fmt.Errorf("error creating Redis broker: %w", err)
		}
	}
	var presenceManager centrifuge.PresenceManager
	if !cfg.PresenceManager.Enabled {
		presenceManager, err = confighelpers.CentrifugeRedisPresenceManager(
			n, cfg.Engine.Redis.Prefix, redisShards, cfg.Engine.Redis.RedisPresenceManagerCommon)
		if err != nil {
			return nil, nil, mode, fmt.Errorf("error creating Redis presence manager: %w", err)
		}
	}
	return broker, presenceManager, mode, nil
}

func createRedisBroker(n *centrifuge.Node, cfgContainer *config.Container) (*centrifuge.RedisBroker, string, error) {
	cfg := cfgContainer.Config()
	redisShards, mode, err := confighelpers.CentrifugeRedisShards(n, cfg.Broker.Redis.Redis)
	if err != nil {
		return nil, "", fmt.Errorf("error creating Redis shards: %w", err)
	}
	broker, err := confighelpers.CentrifugeRedisBroker(
		n, cfg.Broker.Redis.Prefix, redisShards, cfg.Broker.Redis.RedisBrokerCommon, cfg.Broker.Type == "redisnats")
	if err != nil {
		return nil, mode, fmt.Errorf("error creating Redis broker: %w", err)
	}
	return broker, mode, nil
}

func createRedisPresenceManager(n *centrifuge.Node, cfgContainer *config.Container) (centrifuge.PresenceManager, string, error) {
	cfg := cfgContainer.Config()
	redisShards, mode, err := confighelpers.CentrifugeRedisShards(n, cfg.PresenceManager.Redis.Redis)
	if err != nil {
		return nil, "", fmt.Errorf("error creating Redis shards: %w", err)
	}
	presenceManager, err := confighelpers.CentrifugeRedisPresenceManager(
		n, cfg.PresenceManager.Redis.Prefix, redisShards, cfg.PresenceManager.Redis.RedisPresenceManagerCommon)
	if err != nil {
		return nil, mode, fmt.Errorf("error creating Redis presence manager: %w", err)
	}
	return presenceManager, mode, nil
}
