package proxy

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/rs/zerolog/log"
)

// WarnUnknownProxyKeys is a helper to find keys not known by Centrifugo in proxy config.
func WarnUnknownProxyKeys(jsonProxies []byte) {
	var jsonMaps []map[string]interface{}
	err := json.Unmarshal(jsonProxies, &jsonMaps)
	if err != nil {
		log.Warn().Err(err).Msg("error unmarshalling proxies")
		return
	}
	for _, jsonMap := range jsonMaps {
		var data Proxy
		unknownKeys := tools.FindUnknownKeys(jsonMap, data)
		for _, key := range unknownKeys {
			if key == "name" {
				continue
			}
			log.Warn().Str("key", key).Any("proxy_name", jsonMap["name"]).Msg("unknown key found in the proxy object")
		}
	}
}
