package consuming

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/rs/zerolog/log"
)

// WarnUnknownConsumerConfigKeys is a helper to find keys not known by Centrifugo in consumer config.
func WarnUnknownConsumerConfigKeys(jsonConsumers []byte) {
	var jsonMaps []map[string]any
	err := json.Unmarshal(jsonConsumers, &jsonMaps)
	if err != nil {
		log.Warn().Err(err).Msg("error unmarshalling consumers")
		return
	}
	for _, jsonMap := range jsonMaps {
		var data ConsumerConfig
		unknownKeys := tools.FindUnknownKeys(jsonMap, data)
		for _, key := range unknownKeys {
			if key == "name" {
				continue
			}
			log.Warn().Str("key", key).Any("consumer_type", jsonMap["type"]).Msg("unknown key found in the consumer definition object")
		}
		switch jsonMap["type"] {
		case "postgresql":
			configMap, ok := jsonMap["postgresql"].(map[string]any)
			if !ok {
				log.Warn().Any("consumer_type", jsonMap["type"]).Msg("consumer config must be object")
				break
			}
			var data PostgresConfig
			unknownKeys := tools.FindUnknownKeys(configMap, data)
			for _, key := range unknownKeys {
				log.Warn().Str("key", key).Any("consumer_type", jsonMap["type"]).Msg("unknown key found in the consumer config object")
			}
		case "kafka":
			configMap, ok := jsonMap["kafka"].(map[string]any)
			if !ok {
				log.Warn().Any("consumer_type", jsonMap["type"]).Msg("consumer config must be object")
				break
			}
			var data KafkaConfig
			unknownKeys := tools.FindUnknownKeys(configMap, data)
			for _, key := range unknownKeys {
				log.Warn().Str("key", key).Any("consumer_type", jsonMap["type"]).Msg("unknown key found in the consumer config object")
			}
		}
	}
}
