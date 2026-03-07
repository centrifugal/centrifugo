package app

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/confighelpers"
	"github.com/centrifugal/centrifugo/v6/internal/natsbroker"
	"github.com/centrifugal/centrifugo/v6/internal/pgmapbroker"
	"github.com/centrifugal/centrifugo/v6/internal/redisnatsbroker"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
)

func configureEngines(node *centrifuge.Node, cfgContainer *config.Container) error {
	cfg := cfgContainer.Config()

	var broker centrifuge.Broker
	var presenceManager centrifuge.PresenceManager

	if !cfg.Broker.Enabled || !cfg.PresenceManager.Enabled {
		var err error
		var engineMode string
		switch cfg.Engine.Type {
		case "memory":
			broker, presenceManager, err = createMemoryEngine(node)
		case "redis":
			broker, presenceManager, engineMode, err = createRedisEngine(node, cfgContainer)
		default:
			return fmt.Errorf("unknown engine type: %s", cfg.Engine.Type)
		}
		event := log.Info().Str("engine_type", cfg.Engine.Type)
		if engineMode != "" {
			event.Str("engine_mode", engineMode)
		}
		event.Msg("initializing engine")
		if err != nil {
			return fmt.Errorf("error creating engine: %v", err)
		}
	} else {
		log.Info().Msgf("both broker and presence manager enabled, skip engine initialization")
	}

	if cfg.Broker.Enabled {
		var err error
		var brokerMode string
		switch cfg.Broker.Type {
		case "memory":
			broker, err = createMemoryBroker(node)
		case "redis":
			broker, brokerMode, err = createRedisBroker(node, cfgContainer)
		case "nats":
			broker, err = NatsBroker(node, cfg)
			brokerMode = "nats"
		case "redisnats":
			if !cfg.EnableUnreleasedFeatures {
				return fmt.Errorf("redisnats broker requires enable_unreleased_features on")
			}
			log.Warn().Msg("redisnats broker is not released, it may be changed or removed at any point")
			redisBroker, redisBrokerMode, err := createRedisBroker(node, cfgContainer)
			if err != nil {
				return fmt.Errorf("error creating redis broker: %v", err)
			}
			brokerMode = redisBrokerMode + "+nats"
			natsBroker, err := NatsBroker(node, cfg)
			if err != nil {
				return fmt.Errorf("error creating nats broker: %v", err)
			}
			broker, err = redisnatsbroker.New(natsBroker, redisBroker)
			if err != nil {
				return fmt.Errorf("error creating redisnats broker: %v", err)
			}
		default:
			return fmt.Errorf("unknown broker type: %s", cfg.Broker.Type)
		}
		if err != nil {
			return fmt.Errorf("error creating broker: %v", err)
		}
		event := log.Info().Str("broker_type", cfg.Broker.Type)
		if brokerMode != "" {
			event.Str("broker_mode", brokerMode)
		}
		event.Msg("broker is enabled, using it instead of broker from engine")
	} else {
		log.Info().Msgf("explicit broker not provided, using the one from engine")
	}

	if cfg.PresenceManager.Enabled {
		var err error
		var presenceManagerMode string
		switch cfg.PresenceManager.Type {
		case "memory":
			presenceManager, err = createMemoryPresenceManager(node)
		case "redis":
			presenceManager, presenceManagerMode, err = createRedisPresenceManager(node, cfgContainer)
		default:
			return fmt.Errorf("unknown presence manager type: %s", cfg.PresenceManager.Type)
		}
		if err != nil {
			return fmt.Errorf("error creating presence manager: %v", err)
		}
		event := log.Info().Str("presence_manager_type", cfg.PresenceManager.Type)
		if presenceManagerMode != "" {
			event.Str("presence_manager_mode", presenceManagerMode)
		}
		event.Msg("presence manager is enabled, using it instead of presence manager from engine")
	} else {
		log.Info().Msgf("explicit presence manager not provided, using the one from engine")
	}

	node.SetBroker(broker)
	node.SetPresenceManager(presenceManager)
	return nil
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

func configureMapBroker(node *centrifuge.Node, cfgContainer *config.Container) error {
	cfg := cfgContainer.Config()
	var mapBroker centrifuge.MapBroker
	var mapBrokerMode string
	var err error
	switch cfg.MapBroker.Type {
	case "memory":
		mapBroker, err = centrifuge.NewMemoryMapBroker(node, centrifuge.MemoryMapBrokerConfig{})
	case "redis":
		var redisShards []*centrifuge.RedisShard
		redisShards, mapBrokerMode, err = confighelpers.CentrifugeRedisShards(node, cfg.MapBroker.Redis.Redis)
		if err != nil {
			return fmt.Errorf("error creating Redis shards for map broker: %w", err)
		}
		mapBroker, err = confighelpers.CentrifugeRedisMapBroker(
			node, cfg.MapBroker.Redis.Prefix, redisShards, cfg.MapBroker.Redis.RedisMapBrokerCommon)
	case "postgres":
		pgCfg := cfg.MapBroker.Postgres
		pgBrokerCfg := pgmapbroker.PostgresMapBrokerConfig{
			DSN:                 pgCfg.DSN,
			PoolSize:            pgCfg.PoolSize,
			NumShards:           pgCfg.NumShards,
			TTLCheckInterval:    pgCfg.TTLCheckInterval.ToDuration(),
			CleanupInterval:     pgCfg.CleanupInterval.ToDuration(),
			IdempotentResultTTL: pgCfg.IdempotentResultTTL.ToDuration(),
			BinaryData:          pgCfg.BinaryData,
			StreamRetention:     pgCfg.StreamRetention.ToDuration(),
			UseNotify:           pgCfg.UseNotify,
			SkipShardLock:       pgCfg.SkipShardLock,
			Outbox: pgmapbroker.OutboxConfig{
				PollInterval: pgCfg.Outbox.PollInterval.ToDuration(),
				BatchSize:    pgCfg.Outbox.BatchSize,
			},
		}
		mapBroker, err = pgmapbroker.NewPostgresMapBroker(node, pgBrokerCfg)
		if err != nil {
			return fmt.Errorf("error creating Postgres map broker: %w", err)
		}
		if !pgCfg.SkipSchemaInit {
			pgBroker := mapBroker.(*pgmapbroker.PostgresMapBroker)
			if schemaErr := pgBroker.EnsureSchema(context.Background()); schemaErr != nil {
				return fmt.Errorf("error initializing Postgres map broker schema: %w", schemaErr)
			}
		}
		mapBrokerMode = "postgres"
	default:
		return fmt.Errorf("unknown map broker type: %s", cfg.MapBroker.Type)
	}
	if err != nil {
		return fmt.Errorf("error creating map broker: %v", err)
	}
	event := log.Info().Str("map_broker_type", cfg.MapBroker.Type)
	if mapBrokerMode != "" {
		event.Str("map_broker_mode", mapBrokerMode)
	}
	event.Msg("initializing map broker")
	node.SetMapBroker(mapBroker)
	return nil
}
