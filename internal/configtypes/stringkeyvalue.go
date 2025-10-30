package configtypes

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
)

type StringKeyValue map[string]string

func (s *StringKeyValue) Decode(value string) error {
	// Only support slice of key/value objects.
	var kvList []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(value), &kvList); err != nil {
		return fmt.Errorf("cannot decode StringKeyValue: %w", err)
	}

	m := make(map[string]string)
	for i, kv := range kvList {
		if kv.Key == "" {
			return fmt.Errorf("empty key at element %d", i)
		}
		if _, exists := m[kv.Key]; exists {
			return fmt.Errorf("duplicate key %q at element %d", kv.Key, i)
		}
		m[kv.Key] = kv.Value
	}

	if err := expandEnvVars(m); err != nil {
		return err
	}

	*s = m
	return nil
}

func StringToStringKeyValueHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf(StringKeyValue{}) {
			return data, nil
		}

		// Only support slice of key/value objects.
		v, ok := data.([]any)
		if !ok {
			return nil, fmt.Errorf("unsupported type %T for StringKeyValue, expected slice of key/value objects", data)
		}

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
		return StringKeyValue(m), nil
	}
}
