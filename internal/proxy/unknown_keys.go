package proxy

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/rs/zerolog/log"
)

// WarnUnknownProxyKeys is a helper to find keys not known by Centrifugo in proxy config.
func WarnUnknownProxyKeys(jsonProxies []byte) {
	var jsonMaps []map[string]any
	err := json.Unmarshal(jsonProxies, &jsonMaps)
	if err != nil {
		log.Warn().Err(err).Msg("error unmarshalling proxies")
		return
	}
	for _, jsonMap := range jsonMaps {
		var data Config
		unknownKeys := tools.FindUnknownKeys(jsonMap, data)
		for _, key := range unknownKeys {
			if key == "name" {
				continue
			}
			log.Warn().Str("key", key).Any("proxy_name", jsonMap["name"]).Msg("unknown key found in the proxy object")
		}
		tls, ok := jsonMap["grpc_tls"].(map[string]any)
		if ok {
			var TLSConfig tools.TLSConfig
			unknownKeys := tools.FindUnknownKeys(tls, TLSConfig)
			for _, key := range unknownKeys {
				log.Warn().Str("key", key).Any("proxy_name", jsonMap["name"]).Msg("unknown key found in the proxy tls config object")
			}
		}
	}
}
