package centrifugo

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/FZambia/viper-lite"
	"github.com/centrifugal/centrifugo/libcentrifugo/node"
	"github.com/satori/go.uuid"
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
  "secret": "{{.Secret}}"
}
`

var tomlConfigTemplate = `secret = {{.Secret}}
`

var yamlConfigTemplate = `secret: {{.Secret}}
`

// generateConfig generates configuration file at provided path.
func generateConfig(f string) error {
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
	t.Execute(&output, struct {
		Secret string
	}{
		uuid.NewV4().String(),
	})

	err = ioutil.WriteFile(f, output.Bytes(), 0644)
	if err != nil {
		return err
	}

	err = validateConfig(f)
	if err != nil {
		_ = os.Remove(f)
		return err
	}

	return nil
}

// validateConfig validates config file located at provided path.
func validateConfig(f string) error {
	v := viper.New()
	v.SetConfigFile(f)
	err := v.ReadInConfig()
	if err != nil {
		switch err.(type) {
		case viper.ConfigParseError:
			return err
		default:
			return errors.New("Unable to locate config file, use \"centrifugo genconfig -c " + f + "\" command to generate one")
		}
	}
	c := node.NewConfig(v)
	return c.Validate()
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
