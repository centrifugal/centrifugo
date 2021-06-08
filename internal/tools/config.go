package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"text/template"

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

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

var jsonConfigTemplate = `{
  "v3_use_offset": true,
  "token_hmac_secret_key": "{{.TokenSecret}}",
  "admin_password": "{{.AdminPassword}}",
  "admin_secret": "{{.AdminSecret}}",
  "api_key": "{{.APIKey}}",
  "allowed_origins": []
}
`

var tomlConfigTemplate = `v3_use_offset = true
token_hmac_secret_key = "{{.TokenSecret}}"
admin_password = "{{.AdminPassword}}"
admin_secret = "{{.AdminSecret}}"
api_key = "{{.APIKey}}"
allowed_origins = []
`

var yamlConfigTemplate = `v3_use_offset: true
token_hmac_secret_key: {{.TokenSecret}}
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

	supportedExts := []string{"json", "toml", "yaml", "yml"}

	if !stringInSlice(ext, supportedExts) {
		return errors.New("output config file must have one of supported extensions: " + strings.Join(supportedExts, ", "))
	}

	var t *template.Template

	switch ext {
	case "json":
		t, err = template.New("config").Parse(jsonConfigTemplate)
	case "toml":
		t, err = template.New("config").Parse(tomlConfigTemplate)
	case "yaml", "yml":
		t, err = template.New("config").Parse(yamlConfigTemplate)
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
