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
