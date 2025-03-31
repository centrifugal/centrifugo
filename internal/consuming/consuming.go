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
	DispatchCommand(
		ctx context.Context, method string, data []byte,
	) error
	DispatchPublication(
		ctx context.Context, data []byte, idempotencyKey string, delta bool, tags map[string]string, channels ...string,
	) error
}

func New(nodeID string, dispatcher Dispatcher, configs []ConsumerConfig) ([]service.Service, error) {
	metrics := newCommonMetrics(prometheus.DefaultRegisterer)

	var services []service.Service
	for _, config := range configs {
		if !config.Enabled { // Important to keep this check inside specific type for proper config validation.
			log.Info().
				Str("consumer", config.Name).
				Str("type", config.Type).
				Msg("consumer is not enabled, skip")
			continue
		}
		switch config.Type {
		case configtypes.ConsumerTypePostgres:
			consumer, err := NewPostgresConsumer(config.Name, config.Postgres, dispatcher, metrics)
			if err != nil {
				return nil, fmt.Errorf("error initializing PostgreSQL consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		case configtypes.ConsumerTypeKafka:
			consumer, err := NewKafkaConsumer(config.Name, config.Kafka, dispatcher, metrics, nodeID)
			if err != nil {
				return nil, fmt.Errorf("error initializing Kafka consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		case configtypes.ConsumerTypeNatsJetStream:
			consumer, err := NewNatsJetStreamConsumer(config.Name, config.NatsJetStream, dispatcher, metrics)
			if err != nil {
				return nil, fmt.Errorf("error initializing Nats JetStream consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		case configtypes.ConsumerTypeRedisStream:
			consumer, err := NewRedisStreamConsumer(config.Name, config.RedisStream, dispatcher, metrics, nodeID)
			if err != nil {
				return nil, fmt.Errorf("error initializing Redis Stream consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		case configtypes.ConsumerTypeGooglePubSub:
			consumer, err := NewGooglePubSubConsumer(config.Name, config.GooglePubSub, dispatcher, metrics)
			if err != nil {
				return nil, fmt.Errorf("error initializing Google Pub/Sub consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		case configtypes.ConsumerTypeAwsSqs:
			consumer, err := NewAwsSqsConsumer(config.Name, config.AwsSqs, dispatcher, metrics)
			if err != nil {
				return nil, fmt.Errorf("error initializing AWS SNS/SQS consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		case configtypes.ConsumerTypeAzureServiceBus:
			consumer, err := NewAzureServiceBusConsumer(config.Name, config.AzureServiceBus, dispatcher, metrics)
			if err != nil {
				return nil, fmt.Errorf("error initializing Azure Service Bus consumer (%s): %w", config.Name, err)
			}
			services = append(services, consumer)
		default:
			return nil, fmt.Errorf("unknown consumer type: %s", config.Type)
		}
		log.Info().
			Str("consumer", config.Name).
			Str("type", config.Type).
			Msg("running consumer")
	}

	for _, config := range configs {
		metrics.processedTotal.WithLabelValues(config.Name).Add(0)
		metrics.errorsTotal.WithLabelValues(config.Name).Add(0)
	}

	return services, nil
}
