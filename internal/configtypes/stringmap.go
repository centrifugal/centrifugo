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

// expandEnvVars expands CENTRIFUGO_VAR_ environment variables in a map[string]string.
func expandEnvVars(m map[string]string) error {
	// Expand only CENTRIFUGO_VAR_ environment variables in values.
	// This regex specifically matches ${CENTRIFUGO_VAR_*} patterns only.
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
			envVar := match[2 : len(match)-1] // Remove ${ and }.
			return os.Getenv(envVar)
		})
	}
	return nil
}

func (s *MapStringString) Decode(value string) error {
	var m map[string]string
	err := json.Unmarshal([]byte(value), &m)
	if err != nil {
		return err
	}

	err = expandEnvVars(m)
	if err != nil {
		return err
	}

	*s = m
	return nil
}

// StringToMapStringStringHookFunc for mapstructure to decode MapStringString from map[string]any.
func StringToMapStringStringHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		// Only handle map[string]any -> MapStringString conversion.
		if f.Kind() != reflect.Map {
			return data, nil
		}
		if t != reflect.TypeOf(MapStringString{}) {
			return data, nil
		}

		// Convert map[string]any to map[string]string.
		sourceMap, ok := data.(map[string]any)
		if !ok {
			return data, nil
		}

		m := make(map[string]string)
		for key, value := range sourceMap {
			strValue, ok := value.(string)
			if !ok {
				return data, fmt.Errorf("expected value for key %q to be a string, got %T", key, value)
			}
			m[key] = strValue
		}

		// Expand environment variables.
		err := expandEnvVars(m)
		if err != nil {
			return nil, err
		}

		return MapStringString(m), nil
	}
}
