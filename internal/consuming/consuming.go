package consuming

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v5/internal/configtypes"
	"github.com/centrifugal/centrifugo/v5/internal/service"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
)

type ConsumerType string

const (
	ConsumerTypePostgres = "postgresql"
	ConsumerTypeKafka    = "kafka"
)

type ConsumerConfig = configtypes.Consumer

type Dispatcher interface {
	Dispatch(ctx context.Context, method string, data []byte) error
}

type Logger interface {
	LogEnabled(level centrifuge.LogLevel) bool
	Log(node centrifuge.LogEntry)
}

func New(nodeID string, logger Logger, dispatcher Dispatcher, configs []ConsumerConfig) ([]service.Service, error) {
	var services []service.Service
	for _, config := range configs {
		if config.Type == ConsumerTypePostgres {
			if !config.Enabled { // Important to keep this check inside specific type for proper config validation.
				continue
			}
			consumer, err := NewPostgresConsumer(config.Name, logger, dispatcher, config.Postgres)
			if err != nil {
				return nil, fmt.Errorf("error initializing PostgreSQL consumer (%s): %w", config.Name, err)
			}
			log.Info().Str("consumer_name", config.Name).Msg("running consumer")
			services = append(services, consumer)
		} else if config.Type == ConsumerTypeKafka {
			if !config.Enabled {
				continue
			}
			consumer, err := NewKafkaConsumer(config.Name, nodeID, logger, dispatcher, config.Kafka)
			if err != nil {
				return nil, fmt.Errorf("error initializing Kafka consumer (%s): %w", config.Name, err)
			}
			log.Info().Str("consumer_name", config.Name).Msg("running consumer")
			services = append(services, consumer)
		} else {
			return nil, fmt.Errorf("unknown consumer type: %s", config.Type)
		}
	}
	return services, nil
}
