package consuming

import (
	"context"
	"fmt"
	"regexp"

	"github.com/centrifugal/centrifugo/v5/internal/service"

	"github.com/centrifugal/centrifuge"
	"github.com/rs/zerolog/log"
)

type ConsumerType string

const (
	ConsumerTypePostgres = "postgresql"
	ConsumerTypeKafka    = "kafka"
)

type ConsumerConfig struct {
	Name     string          `mapstructure:"name" json:"name"`
	Type     ConsumerType    `mapstructure:"type" json:"type"`
	Enabled  bool            `mapstructure:"enabled" json:"enabled"`
	Postgres *PostgresConfig `mapstructure:"postgresql" json:"postgresql,omitempty"`
	Kafka    *KafkaConfig    `mapstructure:"kafka" json:"kafka,omitempty"`
}

type Dispatcher interface {
	Dispatch(ctx context.Context, method string, data []byte) error
}

type Logger interface {
	LogEnabled(level centrifuge.LogLevel) bool
	Log(node centrifuge.LogEntry)
}

var consumerNamePattern = "^[-a-zA-Z0-9_.]{2,}$"
var consumerNameRe = regexp.MustCompile(consumerNamePattern)

func New(logger Logger, dispatcher Dispatcher, configs []ConsumerConfig) ([]service.Service, error) {
	var services []service.Service
	for _, config := range configs {
		if !consumerNameRe.Match([]byte(config.Name)) {
			log.Fatal().Msgf("invalid consumer name: %s, must match %s regular expression", config.Name, consumerNamePattern)
		}
		if config.Type == ConsumerTypePostgres {
			if !config.Enabled { // Important to keep this check inside specific type for proper config validation.
				continue
			}
			consumer, err := NewPostgresConsumer(logger, dispatcher, *config.Postgres)
			if err != nil {
				return nil, fmt.Errorf("error initializing PostgreSQL consumer: %w", err)
			}
			log.Info().Str("consumer_name", config.Name).Msg("running consumer")
			services = append(services, consumer)
		} else if config.Type == ConsumerTypeKafka {
			if !config.Enabled {
				continue
			}
			consumer, err := NewKafkaConsumer(logger, dispatcher, *config.Kafka)
			if err != nil {
				return nil, fmt.Errorf("error initializing Kafka consumer: %w", err)
			}
			log.Info().Str("consumer_name", config.Name).Msg("running consumer")
			services = append(services, consumer)
		} else {
			return nil, fmt.Errorf("unknown consumer type: %s", config.Type)
		}
	}
	return services, nil
}
