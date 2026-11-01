package consuming

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/api"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/metrics"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ConsumerConfig = configtypes.Consumer

// Consumer consumes messages from an external source.
type Consumer interface {
	// Run consumes until ctx is cancelled.
	Run(ctx context.Context) error
	// Close drains the consumer: messages in flight are handled and
	// acknowledged within ctx (a context of its own, not Run's cancelled one),
	// then what the consumer holds is closed. It is called once Run's ctx is
	// cancelled, also when Run never ran, and may be called more than once.
	Close(ctx context.Context) error
}

type Dispatcher interface {
	DispatchCommand(ctx context.Context, method string, data []byte) error
	DispatchPublication(ctx context.Context, channels []string, pub api.ConsumedPublication) error
}

type consumerCommon struct {
	name   string
	log    zerolog.Logger
	nodeID string
}

// New creates the enabled consumers. On error it closes the consumers it
// already created.
func New(nodeID string, consumingHandler *api.ConsumingHandler, configs []ConsumerConfig) ([]Consumer, error) {
	dispatcher := api.NewDispatcher(consumingHandler)

	var consumers []Consumer
	closeConsumers := func() {
		for _, c := range consumers {
			_ = c.Close(context.Background())
		}
	}
	for _, config := range configs {
		if !config.Enabled { // Important to keep this check inside specific type for proper config validation.
			log.Info().
				Str("consumer", config.Name).
				Str("type", config.Type).
				Msg("consumer is not enabled, skip")
			continue
		}
		common := &consumerCommon{
			name:   config.Name,
			log:    log.With().Str("consumer", config.Name).Logger(),
			nodeID: nodeID,
		}
		// Initialize consumer metrics with zero values.
		metrics.InitConsumerMetrics(config.Name)
		var consumer Consumer
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
			closeConsumers()
			return nil, fmt.Errorf("unknown consumer type: %s", config.Type)
		}
		if err != nil {
			closeConsumers()
			return nil, fmt.Errorf("error initializing %s consumer (%s): %w", config.Type, config.Name, err)
		}
		consumers = append(consumers, consumer)
		metrics.InitConsumerMetrics(config.Name)
		log.Info().
			Str("consumer", config.Name).
			Str("type", config.Type).
			Msg("running consumer")
	}
	return consumers, nil
}
