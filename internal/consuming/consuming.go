package consuming

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

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
	// Name is a unique name required for each consumer.
	Name string `mapstructure:"name" json:"name" envconfig:"name"`

	// Enabled must be true to tell Centrifugo to run configured consumer.
	Enabled bool `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`

	// Type describes the type of consumer.
	Type ConsumerType `mapstructure:"type" json:"type" envconfig:"type"`

	// Postgres allows defining options for consumer of postgresql type.
	Postgres *PostgresConfig `mapstructure:"postgresql" json:"postgresql,omitempty" envconfig:"postgresql"`
	// Kafka allows defining options for consumer of kafka type.
	Kafka *KafkaConfig `mapstructure:"kafka" json:"kafka,omitempty" envconfig:"kafka"`
}

type Dispatcher interface {
	Dispatch(ctx context.Context, method string, data []byte) error
}

type Logger interface {
	LogEnabled(level centrifuge.LogLevel) bool
	Log(node centrifuge.LogEntry)
}

var consumerNamePattern = "^[a-zA-Z0-9_]{2,}$"
var consumerNameRe = regexp.MustCompile(consumerNamePattern)

func New(nodeID string, logger Logger, dispatcher Dispatcher, configs []ConsumerConfig) ([]service.Service, error) {
	var services []service.Service
	var names []string
	for _, config := range configs {
		if !consumerNameRe.Match([]byte(config.Name)) {
			log.Fatal().Msgf("invalid consumer name: %s, must match %s regular expression", config.Name, consumerNamePattern)
		}
		if slices.Contains(names, config.Name) {
			log.Fatal().Msgf("invalid consumer name: %s, must be unique", config.Name)
		}
		names = append(names, config.Name)
		if config.Type == ConsumerTypePostgres {
			if !config.Enabled { // Important to keep this check inside specific type for proper config validation.
				continue
			}
			if config.Postgres == nil {
				config.Postgres = &PostgresConfig{}
			}
			dsn := os.Getenv("CENTRIFUGO_CONSUMERS_POSTGRESQL_" + strings.ToUpper(config.Name) + "_DSN")
			if dsn != "" {
				config.Postgres.DSN = dsn
			}
			consumer, err := NewPostgresConsumer(config.Name, logger, dispatcher, *config.Postgres)
			if err != nil {
				return nil, fmt.Errorf("error initializing PostgreSQL consumer (%s): %w", config.Name, err)
			}
			log.Info().Str("consumer_name", config.Name).Msg("running consumer")
			services = append(services, consumer)
		} else if config.Type == ConsumerTypeKafka {
			if !config.Enabled {
				continue
			}
			if config.Kafka == nil {
				config.Kafka = &KafkaConfig{}
			}
			brokers := os.Getenv("CENTRIFUGO_CONSUMERS_KAFKA_" + strings.ToUpper(config.Name) + "_BROKERS")
			if brokers != "" {
				config.Kafka.Brokers = strings.Split(brokers, " ")
			}
			saslUser := os.Getenv("CENTRIFUGO_CONSUMERS_KAFKA_" + strings.ToUpper(config.Name) + "_SASL_USER")
			if saslUser != "" {
				config.Kafka.SASLUser = saslUser
			}
			saslPassword := os.Getenv("CENTRIFUGO_CONSUMERS_KAFKA_" + strings.ToUpper(config.Name) + "_SASL_PASSWORD")
			if saslPassword != "" {
				config.Kafka.SASLPassword = saslPassword
			}
			consumer, err := NewKafkaConsumer(config.Name, nodeID, logger, dispatcher, *config.Kafka)
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
