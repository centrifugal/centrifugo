package tools

import (
	"os"
	"reflect"
	"strings"

	"github.com/hashicorp/go-envparse"
	"github.com/rs/zerolog/log"
)

// FindUnknownKeys returns a list of keys from map not existing in struct JSON tags.
func FindUnknownKeys[T any](jsonMap map[string]any, data T) []string {
	jsonKeys := make(map[string]bool)
	for key := range jsonMap {
		jsonKeys[key] = true
	}

	structKeys := getStructJSONTags(reflect.TypeOf(data))

	unknownKeys := make([]string, 0)
	for key := range jsonKeys {
		if !structKeys[key] {
			unknownKeys = append(unknownKeys, key)
		}
	}

	return unknownKeys
}

// getStructJSONTags returns a map of JSON tags found in the struct (including embedded structs).
func getStructJSONTags(t reflect.Type) map[string]bool {
	structKeys := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Check if the field is an embedded struct and recursively get its fields.
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			embeddedFields := getStructJSONTags(field.Type)
			for key := range embeddedFields {
				structKeys[key] = true
			}
			continue
		}

		// Extract the JSON tag.
		jsonKey := strings.TrimSuffix(field.Tag.Get("json"), ",omitempty")
		if jsonKey != "" {
			structKeys[jsonKey] = true
		}
	}

	return structKeys
}

// CheckPlainConfigKeys warns users about unknown keys in the configuration. It does not
// include warnings about some arrays we have in the configuration - like namespaces or proxies
// configuration. Those are checked separately on decoding.
func CheckPlainConfigKeys(defaults map[string]any, allKeys []string) {
	checkFileConfigKeys(defaults, allKeys)
	checkEnvironmentConfigKeys(defaults)
}

// Map[string]string keys require a special care because viper will return all the keys found
// in map from allKeys method in a format like "proxy_static_http_headers.some_key". Since we
// allow arbitrary keys for maps we have this slice of such configuration options here.
var mapStringStringKeys = []string{
	"proxy_static_http_headers",
}

func isMapStringStringKey(key string) bool {
	for _, mapKey := range mapStringStringKeys {
		if strings.HasPrefix(key, mapKey+".") {
			return true
		}
	}
	return false
}

func checkFileConfigKeys(defaults map[string]any, allKeys []string) {
	for _, key := range allKeys {
		if isMapStringStringKey(key) {
			continue
		}
		if _, ok := defaults[key]; !ok {
			log.Warn().Str("key", key).Msg("unknown key found in the configuration file")
		}
	}
}

func checkEnvironmentConfigKeys(defaults map[string]any) {
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
			// Also, there are env vars inside more complex config structures we also want to
			// not warn about.
			if isKnownEnvVarPrefix(envKey) {
				continue
			}
			if !isKnownEnvKey(defaults, envPrefix, envKey) {
				log.Warn().Str("key", envKey).Msg("unknown key found in the environment")
			}
		}
	}
}

var knownPrefixes = []string{
	"CENTRIFUGO_CONSUMERS_",
}

func isKnownEnvVarPrefix(envKey string) bool {
	for _, prefix := range knownPrefixes {
		if strings.HasPrefix(envKey, prefix) {
			return true
		}
	}
	return false
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

func isKnownEnvKey(defaults map[string]any, envPrefix string, envKey string) bool {
	for defKey := range defaults {
		defKeyEnvFormat := envPrefix + strings.ToUpper(strings.ReplaceAll(defKey, ".", "_"))
		if defKeyEnvFormat == envKey {
			return true
		}
	}
	return false
}
