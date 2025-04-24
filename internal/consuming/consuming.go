package consuming

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/service"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ConsumerConfig = configtypes.Consumer

type Dispatcher interface {
	DispatchCommand(ctx context.Context, method string, data []byte) error
	DispatchPublication(ctx context.Context, channels []string, pub api.ConsumedPublication) error
}

type consumerCommon struct {
	name    string
	log     zerolog.Logger
	metrics *commonMetrics
	nodeID  string
}

func New(nodeID string, consumingHandler *api.ConsumingHandler, configs []ConsumerConfig) ([]service.Service, error) {
	metrics := newCommonMetrics(prometheus.DefaultRegisterer)
	dispatcher := api.NewDispatcher(consumingHandler)

	var services []service.Service
	for _, config := range configs {
		if !config.Enabled { // Important to keep this check inside specific type for proper config validation.
			log.Info().
				Str("consumer", config.Name).
				Str("type", config.Type).
				Msg("consumer is not enabled, skip")
			continue
		}
		common := &consumerCommon{
			name:    config.Name,
			log:     log.With().Str("consumer", config.Name).Logger(),
			metrics: metrics,
			nodeID:  nodeID,
		}
		var consumer service.Service
		var err error
		switch config.Type {
		case configtypes.ConsumerTypePostgres:
			consumer, err = NewPostgresConsumer(config.Postgres, dispatcher, common)
		case configtypes.ConsumerTypeKafka:
			consumer, err = NewKafkaConsumer(config.Kafka, dispatcher, common)
		case configtypes.ConsumerTypeNatsJetStream:
			consumer, err = NewNatsJetStreamConsumer(config.NatsJetStream, dispatcher, common)
		case configtypes.ConsumerTypeRedisStream:
			consumer, err = NewRedisStreamConsumer(config.RedisStream, dispatcher, common)
		case configtypes.ConsumerTypeGooglePubSub:
			consumer, err = NewGooglePubSubConsumer(config.GooglePubSub, dispatcher, common)
		case configtypes.ConsumerTypeAwsSqs:
			consumer, err = NewAwsSqsConsumer(config.AwsSqs, dispatcher, common)
		case configtypes.ConsumerTypeAzureServiceBus:
			consumer, err = NewAzureServiceBusConsumer(config.AzureServiceBus, dispatcher, common)
		default:
			return nil, fmt.Errorf("unknown consumer type: %s", config.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("error initializing %s consumer (%s): %w", config.Type, config.Name, err)
		}
		services = append(services, consumer)
		metrics.init(config.Name)
		log.Info().
			Str("consumer", config.Name).
			Str("type", config.Type).
			Msg("running consumer")
	}
	return services, nil
}
