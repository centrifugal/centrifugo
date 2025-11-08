package configtypes

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
)

type StringKeyValue struct {
	// Key is the key of the key/value pair. Must be unique within a StringKeyValues list.
	Key string `mapstructure:"key" json:"key" envconfig:"key" yaml:"key" toml:"key"`
	// Value is the value of the key/value pair.
	Value string `mapstructure:"value" json:"value" envconfig:"value" yaml:"value" toml:"value"`
}

type StringKeyValues []StringKeyValue

// ToMap converts StringKeyValues to a map[string]string for easier lookups.
func (s *StringKeyValues) ToMap() map[string]string {
	if s == nil {
		return nil
	}
	m := make(map[string]string, len(*s))
	for _, kv := range *s {
		m[kv.Key] = kv.Value
	}
	return m
}

func (s *StringKeyValues) Decode(value string) error {
	// Try decoding as map[string]string first (simpler syntax).
	var m map[string]string
	if err := json.Unmarshal([]byte(value), &m); err == nil {
		// Convert map to slice, applying env var expansion
		if err := expandEnvVars(m); err != nil {
			return err
		}
		result := make([]StringKeyValue, 0, len(m))
		for k, v := range m {
			result = append(result, StringKeyValue{Key: k, Value: v})
		}
		*s = result
		return nil
	}

	// If that fails, try decoding as slice of key/value objects.
	var kvList []StringKeyValue
	if err := json.Unmarshal([]byte(value), &kvList); err != nil {
		return fmt.Errorf("cannot decode StringKeyValues: %w", err)
	}

	// Validate and apply env var expansion
	m2 := make(map[string]string)
	for i, kv := range kvList {
		if _, exists := m2[kv.Key]; exists {
			return fmt.Errorf("duplicate key %q at element %d", kv.Key, i)
		}
		m2[kv.Key] = kv.Value
	}

	if err := expandEnvVars(m2); err != nil {
		return err
	}

	// Update values with expanded env vars
	for i := range kvList {
		kvList[i].Value = m2[kvList[i].Key]
	}

	*s = kvList
	return nil
}

func StringToStringKeyValuesHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf(StringKeyValues{}) {
			return data, nil
		}

		// Only support slice of key/value objects.
		v, ok := data.([]any)
		if !ok {
			return nil, fmt.Errorf("unsupported type %T for StringKeyValues, expected slice of key/value objects", data)
		}

		result := make([]StringKeyValue, 0, len(v))
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
			result = append(result, StringKeyValue{Key: keyI, Value: valI})
		}

		// Apply env var expansion
		if err := expandEnvVars(m); err != nil {
			return nil, err
		}

		// Update values with expanded env vars
		for i := range result {
			result[i].Value = m[result[i].Key]
		}

		return StringKeyValues(result), nil
	}
}
