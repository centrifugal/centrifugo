package consuming

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/service"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

type ConsumerConfig = configtypes.Consumer

type Dispatcher interface {
	DispatchAPICommand(ctx context.Context, mode configtypes.ConsumerContentMode, method string, data []byte) error
	DispatchPublication(ctx context.Context, data []byte, idempotencyKey string, delta bool, channels ...string) error
}

func New(nodeID string, dispatcher Dispatcher, configs []ConsumerConfig) ([]service.Service, error) {
	metrics := newCommonMetrics(prometheus.DefaultRegisterer)

	var services []service.Service
	for _, config := range configs {
		if !config.Enabled { // Important to keep this check inside specific type for proper config validation.
			log.Info().Str("consumer_name", config.Name).Str("consumer_type", config.Type).
				Msg("consumer is not enabled, skip")
			continue
		}
		switch config.Type {
		case configtypes.ConsumerTypePostgres:
			consumer, err := NewPostgresConsumer(config.Name, config.ContentMode, dispatcher, config.Postgres, metrics)
			if err != nil {
				return nil, fmt.Errorf("error initializing PostgreSQL consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		case configtypes.ConsumerTypeKafka:
			consumer, err := NewKafkaConsumer(config.Name, config.ContentMode, nodeID, dispatcher, config.Kafka, metrics)
			if err != nil {
				return nil, fmt.Errorf("error initializing Kafka consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		default:
			return nil, fmt.Errorf("unknown consumer type: %s", config.Type)
		}
		log.Info().Str("consumer_name", config.Name).Str("consumer_type", config.Type).Msg("running consumer")
	}

	for _, config := range configs {
		metrics.processedTotal.WithLabelValues(config.Name).Add(0)
		metrics.errorsTotal.WithLabelValues(config.Name).Add(0)
	}

	return services, nil
}
