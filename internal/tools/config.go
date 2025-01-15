package tools

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

// PathExists returns whether the given file or directory exists or not
func PathExists(path string) (bool, error) {
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
  "client": {
    "token": {
      "hmac_secret_key": "{{.TokenSecret}}"
    },
    "allowed_origins": []
  },
  "admin": {
    "enabled": false,
    "password": "{{.AdminPassword}}",
    "secret": "{{.AdminSecret}}"
  },
  "http_api": {
    "key": "{{.APIKey}}"
  }
}
`

var tomlConfigTemplate = `[client]
  allowed_origins = []

  [client.token]
    hmac_secret_key = "{{.TokenSecret}}"

[admin]
  enabled = false
  password = "{{.AdminPassword}}"
  secret = "{{.AdminSecret}}"

[http_api]
  key = "{{.APIKey}}"
`

var yamlConfigTemplate = `client:
  token:
    hmac_secret_key: "{{.TokenSecret}}"
  allowed_origins: []

admin:
  enabled: false
  password: "{{.AdminPassword}}"
  secret: "{{.AdminSecret}}"

http_api:
  key: "{{.APIKey}}"
`

// GenerateConfig generates configuration file at provided path.
func GenerateConfig(f string) error {
	exists, err := PathExists(f)
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
		mustGenerateSecretKey(64),
		mustGenerateSecretKey(16),
		mustGenerateSecretKey(64),
		mustGenerateSecretKey(64),
	})

	return os.WriteFile(f, output.Bytes(), 0644)
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

func mustGenerateSecretKey(byteLen int) string {
	key := make([]byte, byteLen)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(key)
}
