package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/FZambia/viper-lite"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"
)

func DecodeSlice(v *viper.Viper, dst any, key string) []byte {
	var jsonData []byte
	var err error
	switch val := v.Get(key).(type) {
	case string:
		jsonData = []byte(val)
		err = json.Unmarshal([]byte(val), dst)
	case []any:
		jsonData, err = json.Marshal(translateMap(val))
		if err != nil {
			log.Fatal().Err(err).Msgf("error marshalling config %s slice", key)
		}
		decoderCfg := DecoderConfig(dst)
		decoder, newErr := mapstructure.NewDecoder(decoderCfg)
		if newErr != nil {
			log.Fatal().Msg(newErr.Error())
		}
		err = decoder.Decode(v.Get(key))
	default:
		err = fmt.Errorf("unknown %s type: %T", key, val)
	}
	if err != nil {
		log.Fatal().Err(err).Msgf("malformed %s", key)
	}
	return jsonData
}

// translateMap is a helper to deal with map[any]any which YAML uses when unmarshalling.
// We always use string keys and not making this transform results into errors on JSON marshaling.
func translateMap(input []any) []map[string]any {
	var result []map[string]any
	for _, elem := range input {
		switch v := elem.(type) {
		case map[any]any:
			translatedMap := make(map[string]any)
			for key, value := range v {
				stringKey := fmt.Sprintf("%v", key)
				translatedMap[stringKey] = value
			}
			result = append(result, translatedMap)
		case map[string]any:
			result = append(result, v)
		default:
			log.Fatal().Msgf("invalid type in slice: %T", elem)
		}
	}
	return result
}

// DecoderConfig returns default mapstructure.DecoderConfig with support
// of time.Duration values & string slices & Duration
func DecoderConfig(output any) *mapstructure.DecoderConfig {
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

// StringToDurationHookFunc returns a DecodeHookFunc that converts
// strings to time.Duration.
func StringToDurationHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data any) (any, error) {
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
	var v any
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
