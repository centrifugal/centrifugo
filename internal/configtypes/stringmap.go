package configtypes

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"

	"github.com/go-viper/mapstructure/v2"
)

type MapStringString map[string]string

var customEnvVarRegex = regexp.MustCompile(`\$\{(CENTRIFUGO_VAR_[^}]+)}`)

func expandEnvVars(m map[string]string) error {
	for key, val := range m {
		// First check if all CENTRIFUGO_VAR_ environment variables exist.
		matches := customEnvVarRegex.FindAllStringSubmatch(val, -1)
		for _, match := range matches {
			if len(match) > 1 {
				envVar := match[1]

				// Check if the environment variable exists.
				if _, exists := os.LookupEnv(envVar); !exists {
					return fmt.Errorf("environment variable %q not found", envVar)
				}
			}
		}

		// If all variables exist, do the replacement.
		m[key] = customEnvVarRegex.ReplaceAllStringFunc(val, func(match string) string {
			submatches := customEnvVarRegex.FindStringSubmatch(match)
			if len(submatches) > 1 {
				return os.Getenv(submatches[1])
			}
			return match // Fallback, shouldn't happen.
		})
	}
	return nil
}

func (s *MapStringString) Decode(value string) error {
	// Try decoding as map[string]string (old behavior).
	var m map[string]string
	if err := json.Unmarshal([]byte(value), &m); err == nil {
		if err := expandEnvVars(m); err != nil {
			return err
		}
		*s = m
		return nil
	}

	// If that fails, try decoding as slice of key/value objects.
	var kvList []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(value), &kvList); err != nil {
		return fmt.Errorf("cannot decode MapStringString: %w", err)
	}

	m2 := make(map[string]string)
	for i, kv := range kvList {
		if kv.Key == "" {
			return fmt.Errorf("empty key at element %d", i)
		}
		if _, exists := m2[kv.Key]; exists {
			return fmt.Errorf("duplicate key %q at element %d", kv.Key, i)
		}
		m2[kv.Key] = kv.Value
	}

	if err := expandEnvVars(m2); err != nil {
		return err
	}

	*s = m2
	return nil
}

func StringToMapStringStringHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf(MapStringString{}) {
			return data, nil
		}

		switch v := data.(type) {

		// Old behavior: map[string]any – it's case-insensitive, and does not support dot (key delimiter in Viper) in the key.
		case map[string]any:
			m := make(map[string]string)
			for key, value := range v {
				strValue, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("expected string value for key %q, got %T", key, value)
				}
				m[key] = strValue
			}
			if err := expandEnvVars(m); err != nil {
				return nil, err
			}
			return MapStringString(m), nil

		// Slice of key/value objects is the recommended way to define maps now.
		case []any:
			m := make(map[string]string)
			for i, item := range v {
				kvMap, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("expected map for element %d, got %T", i, item)
				}

				keyI, ok := kvMap["key"].(string)
				if !ok {
					return nil, fmt.Errorf("missing or invalid key in element %d", i)
				}

				if _, exists := m[keyI]; exists {
					return nil, fmt.Errorf("duplicate key %q at element %d", keyI, i)
				}

				valI, ok := kvMap["value"].(string)
				if !ok {
					return nil, fmt.Errorf("missing or invalid value in element %d", i)
				}

				m[keyI] = valI
			}
			if err := expandEnvVars(m); err != nil {
				return nil, err
			}
			return MapStringString(m), nil

		default:
			return nil, fmt.Errorf("unsupported type %T for MapStringString", data)
		}
	}
}
