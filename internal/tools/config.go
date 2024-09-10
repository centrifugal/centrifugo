package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/FZambia/viper-lite"
	"github.com/google/uuid"
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

func OptionalStringChoice(value string, choices []string) (string, error) {
	if value == "" {
		// Empty value is valid for optional configuration key.
		return value, nil
	}
	if !slices.Contains(choices, value) {
		return "", fmt.Errorf("invalid value: %s, possible choices are: %s", value, strings.Join(choices, ", "))
	}
	return value, nil
}
