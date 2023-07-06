package tools

import (
	"os"
	"reflect"
	"strings"

	"github.com/hashicorp/go-envparse"
	"github.com/rs/zerolog/log"
)

// FindUnknownKeys returns a list of keys from map not existing in struct JSON tags.
func FindUnknownKeys[T any](jsonMap map[string]interface{}, data T) []string {
	jsonKeys := make(map[string]bool)
	for key := range jsonMap {
		jsonKeys[key] = true
	}

	structKeys := make(map[string]bool)
	s := reflect.ValueOf(data)
	typeOfStruct := s.Type()

	for i := 0; i < s.NumField(); i++ {
		field := typeOfStruct.Field(i)
		// Name extraction may be further improved but enough for our structs.
		jsonKey := strings.TrimSuffix(field.Tag.Get("json"), ",omitempty")
		structKeys[jsonKey] = true
	}

	unknownKeys := make([]string, 0)
	for key := range jsonKeys {
		if !structKeys[key] {
			unknownKeys = append(unknownKeys, key)
		}
	}

	return unknownKeys
}

// CheckPlainConfigKeys warns users about unknown keys in the configuration. It does not
// include warnings about some arrays we have in the configuration - like namespaces or proxies
// configuration. Those are checked separately on decoding.
func CheckPlainConfigKeys(defaults map[string]interface{}, allKeys []string) {
	checkFileConfigKeys(defaults, allKeys)
	checkEnvironmentConfigKeys(defaults)
}

func checkFileConfigKeys(defaults map[string]interface{}, allKeys []string) {
	for _, key := range allKeys {
		if _, ok := defaults[key]; !ok {
			log.Warn().Str("key", key).Msg("unknown key found in the configuration file")
		}
	}
}

func checkEnvironmentConfigKeys(defaults map[string]interface{}) {
	envPrefix := "CENTRIFUGO_"
	envVars := os.Environ()

	for _, envVar := range envVars {
		kv, err := envparse.Parse(strings.NewReader(envVar))
		if err != nil {
			continue
		}
		for envKey := range kv {
			if !strings.HasPrefix(envKey, envPrefix) {
				continue
			}
			// Kubernetes automatically adds some variables which are not used by Centrifugo
			// itself. We skip warnings about them.
			if isKubernetesEnvVar(envKey) {
				continue
			}
			if !isKnownEnvKey(defaults, envPrefix, envKey) {
				log.Warn().Str("key", envKey).Msg("unknown key found in the environment")
			}
		}
	}
}

var k8sPrefixes = []string{
	"CENTRIFUGO_PORT_",
	"CENTRIFUGO_SERVICE_",
}

func isKubernetesEnvVar(envKey string) bool {
	for _, k8sPrefix := range k8sPrefixes {
		if strings.HasPrefix(envKey, k8sPrefix) {
			return true
		}
	}
	return false
}

func isKnownEnvKey(defaults map[string]interface{}, envPrefix string, envKey string) bool {
	for defKey := range defaults {
		defKeyEnvFormat := envPrefix + strings.ToUpper(strings.ReplaceAll(defKey, ".", "_"))
		if defKeyEnvFormat == envKey {
			return true
		}
	}
	return false
}
