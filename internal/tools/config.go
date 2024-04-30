package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/FZambia/viper-lite"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"
)

// pathExists returns whether the given file or directory exists or not
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

var jsonConfigTemplate = `{
  "token_hmac_secret_key": "{{.TokenSecret}}",
  "admin_password": "{{.AdminPassword}}",
  "admin_secret": "{{.AdminSecret}}",
  "api_key": "{{.APIKey}}",
  "allowed_origins": []
}
`

var tomlConfigTemplate = `token_hmac_secret_key = "{{.TokenSecret}}"
admin_password = "{{.AdminPassword}}"
admin_secret = "{{.AdminSecret}}"
api_key = "{{.APIKey}}"
allowed_origins = []
`

var yamlConfigTemplate = `token_hmac_secret_key: {{.TokenSecret}}
admin_password: {{.AdminPassword}}
admin_secret: {{.AdminSecret}}
api_key: {{.APIKey}}
allowed_origins: []
`

// GenerateConfig generates configuration file at provided path.
func GenerateConfig(f string) error {
	exists, err := pathExists(f)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("output config file already exists: " + f)
	}
	ext := filepath.Ext(f)

	if len(ext) > 1 {
		ext = ext[1:]
	}

	supportedExtensions := []string{"json", "toml", "yaml", "yml"}

	var t *template.Template

	switch ext {
	case "json":
		t, err = template.New("config").Parse(jsonConfigTemplate)
	case "toml":
		t, err = template.New("config").Parse(tomlConfigTemplate)
	case "yaml", "yml":
		t, err = template.New("config").Parse(yamlConfigTemplate)
	default:
		return errors.New("output config file must have one of supported extensions: " + strings.Join(supportedExtensions, ", "))
	}
	if err != nil {
		return err
	}

	var output bytes.Buffer
	_ = t.Execute(&output, struct {
		TokenSecret   string
		AdminPassword string
		AdminSecret   string
		APIKey        string
	}{
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
	})

	return os.WriteFile(f, output.Bytes(), 0644)
}

// ErrorMessageFromConfigError tries building a more human-friendly error
// from a configuration error. At the moment we can additionally extract
// JSON syntax error line and column.
// Related issue: https://github.com/golang/go/issues/43513.
func ErrorMessageFromConfigError(err error, configPath string) string {
	var configParseError viper.ConfigParseError
	if ok := errors.As(err, &configParseError); ok {
		var syntaxErr *json.SyntaxError
		if ok := errors.As(configParseError.Err, &syntaxErr); ok {
			if content, readFileErr := os.ReadFile(configPath); readFileErr == nil {
				offset := int(syntaxErr.Offset)
				line := 1 + bytes.Count(content[:offset], []byte("\n"))
				column := offset - (bytes.LastIndex(content[:offset], []byte("\n")) + len("\n"))
				return fmt.Sprintf(
					"JSON syntax error around line %d, column %d: %s",
					line, column, syntaxErr.Error(),
				)
			}
		}
	}
	// Fallback if we can't construct a better one.
	return fmt.Sprintf("configuration error: %v", err)
}

func MapStringString(v *viper.Viper, key string) (map[string]string, error) {
	if !v.IsSet(key) {
		return map[string]string{}, nil
	}
	var m map[string]string
	var err error
	switch val := v.Get(key).(type) {
	case string:
		err = json.Unmarshal([]byte(val), &m)
	case map[string]string:
		m = val
	case map[string]any:
		var jsonData []byte
		jsonData, err = json.Marshal(val)
		if err == nil {
			err = json.Unmarshal(jsonData, &m)
		}
	default:
		err = fmt.Errorf("unknown type: %T", val)
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func OptionalStringChoice(v *viper.Viper, key string, choices []string) (string, error) {
	val := v.GetString(key)
	if val == "" {
		// Empty value is valid for optional configuration key.
		return val, nil
	}
	if !stringInSlice(val, choices) {
		return "", fmt.Errorf("invalid value for %s: %s, possible choices are: %s", key, val, strings.Join(choices, ", "))
	}
	return val, nil
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
