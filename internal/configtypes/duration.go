package configtypes

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
)

type Duration time.Duration

func (d Duration) String() string {
	return d.ToDuration().String()
}

// ToDuration converts the Duration type to time.Duration.
func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}

// UnmarshalJSON interprets numeric values as nanoseconds (default behavior),
// and strings using time.ParseDuration.
func (d *Duration) UnmarshalJSON(b []byte) error {
	// Check if it's a string (starts with a quote).
	if len(b) > 0 && b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		parsed, err := time.ParseDuration(str)
		if err != nil {
			return err
		}
		*d = Duration(parsed)
		return nil
	}
	// Otherwise assume it's a numeric value in nanoseconds.
	var ns int64
	if err := json.Unmarshal(b, &ns); err != nil {
		return err
	}
	*d = Duration(ns)
	return nil
}

// MarshalJSON for JSON encoding.
func (d Duration) MarshalJSON() ([]byte, error) {
	durationStr := time.Duration(d).String()
	return json.Marshal(durationStr)
}

// MarshalText for TOML encoding.
func (d Duration) MarshalText() ([]byte, error) {
	durationStr := time.Duration(d).String()
	return []byte(durationStr), nil
}

// MarshalYAML for YAML encoding.
func (d Duration) MarshalYAML() (interface{}, error) {
	durationStr := time.Duration(d).String()
	return durationStr, nil
}

func StringToDurationHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(Duration(5)) {
			return data, nil
		}

		// Convert it by parsing
		return time.ParseDuration(data.(string))
	}
}
