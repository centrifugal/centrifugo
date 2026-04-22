package controllers

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
)

// New creates and initializes a controller from the given config.
// Returns nil, nil when config.Enabled is false.
func New(node *centrifuge.Node, config configtypes.Controller) (centrifuge.Controller, error) {
	if !config.Enabled {
		return nil, nil
	}

	if config.Type == "postgres" {
		controller, err := NewPostgresController(node, PostgresControllerConfig{
			DSN:                      config.Postgres.DSN,
			TLS:                      config.Postgres.TLS,
			PoolSize:                 config.Postgres.PoolSize,
			NumShards:                config.Postgres.NumShards,
			TablePrefix:              config.Postgres.TablePrefix,
			PollInterval:             config.Postgres.PollInterval.ToDuration(),
			UseNotify:                config.Postgres.UseNotify,
			NotifyDSN:                config.Postgres.NotifyDSN,
			PartitionRetentionDays:   config.Postgres.PartitionRetentionDays,
			PartitionLookaheadDays:   config.Postgres.PartitionLookaheadDays,
			PartitionCleanupInterval: config.Postgres.PartitionCleanupInterval.ToDuration(),
			SkipSchemaInit:           config.Postgres.SkipSchemaInit,
		})
		if err != nil {
			return nil, fmt.Errorf("error initializing Postgres controller: %w", err)
		}
		if !config.Postgres.SkipSchemaInit {
			if err := controller.EnsureSchema(context.Background()); err != nil {
				return nil, fmt.Errorf("error initializing Postgres controller schema: %w", err)
			}
		}
		log.Info().Str("controller_type", config.Type).Msg("controller is enabled")
		return controller, nil
	}

	return nil, fmt.Errorf("unknown controller type: %s (OSS supports \"postgres\" only; Redis and Nats controllers are available in Centrifugo PRO)", config.Type)
}
