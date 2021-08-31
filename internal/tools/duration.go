package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/mitchellh/mapstructure"
)

// DecoderConfig returns default mapstructure.DecoderConfig with support
// of time.Duration values & string slices & Duration
func DecoderConfig(output interface{}) *mapstructure.DecoderConfig {
	return &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			StringToDurationHookFunc(),
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	}
}

type Duration time.Duration

// StringToTimeDurationHookFunc returns a DecodeHookFunc that converts
// strings to time.Duration.
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

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		duration := time.Duration(value)
		if duration > 0 && duration < time.Millisecond {
			return fmt.Errorf("malformed duration: %s, minimal duration resolution is 1ms – make sure correct time unit set", duration)
		}
		*d = Duration(duration)
		return nil
	case string:
		duration, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		if duration > 0 && duration < time.Millisecond {
			return fmt.Errorf("malformed duration: %s, minimal duration resolution is 1ms – make sure correct time unit set", duration)
		}
		*d = Duration(duration)
		return nil
	default:
		return errors.New("invalid duration")
	}
}
