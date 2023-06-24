package rule

import (
	"encoding/json"

	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/rs/zerolog/log"
)

func WarnUnknownNamespaceKeys(jsonNamespaces []byte) {
	var jsonMaps []map[string]interface{}
	err := json.Unmarshal(jsonNamespaces, &jsonMaps)
	if err != nil {
		log.Warn().Err(err).Msg("error unmarshalling namespaces")
		return
	}

	for _, jsonMap := range jsonMaps {
		var data ChannelOptions
		unknownKeys := tools.FindUnknownKeys(jsonMap, data)
		for _, key := range unknownKeys {
			if key == "name" {
				continue
			}
			log.Warn().Str("key", key).Any("namespace_name", jsonMap["name"]).Msg("unknown key found in the namespace object")
		}
	}
}

func WarnUnknownRpcNamespaceKeys(jsonRpcNamespaces []byte) {
	var jsonMaps []map[string]interface{}
	err := json.Unmarshal(jsonRpcNamespaces, &jsonMaps)
	if err != nil {
		log.Warn().Err(err).Msg("error unmarshalling rpc namespaces")
		return
	}

	for _, jsonMap := range jsonMaps {
		var data RpcOptions
		unknownKeys := tools.FindUnknownKeys(jsonMap, data)
		for _, key := range unknownKeys {
			if key == "name" {
				continue
			}
			log.Warn().Str("key", key).Any("rpc_namespace_name", jsonMap["name"]).Msg("unknown key found in the rpc namespace object")
		}
	}
}
