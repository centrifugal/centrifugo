package config

import (
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

func applyConfigMigrations(cfg Config) (Config, []string) {
	var warnings []string
	for i, consumer := range cfg.Consumers {
		if consumer.Type == configtypes.ConsumerTypeKafka {
			if consumer.Kafka.PublicationDataMode.Enabled {
				warnings = append(warnings, fmt.Sprintf("consumers[%d].kafka.publication_data_mode.enabled is deprecated "+
					"and will be removed in future releases, set consumers[%d].content_mode to 'publication_data' instead", i, i))
				cfg.Consumers[i].ContentMode = configtypes.ConsumerContentModePublicationData
			}
		}
	}
	return cfg, warnings
}
