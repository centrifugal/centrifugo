package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func DefaultConfigCommand() *cobra.Command {
	var defaultConfigFile string
	var defaultConfigCmd = &cobra.Command{
		Use:   "defaultconfig",
		Short: "Generate full configuration file with defaults",
		Long:  `Generate full Centrifugo configuration file with defaults`,
		Run: func(cmd *cobra.Command, args []string) {
			DefaultConfig(defaultConfigFile)
		},
	}
	defaultConfigCmd.Flags().StringVarP(&defaultConfigFile, "config", "c", "config.json", "path to default config file to generate")
	return defaultConfigCmd
}

func DefaultConfig(configFile string) {
	exists, err := tools.PathExists(configFile)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if exists {
		fmt.Printf("error: target file already exists\n")
		os.Exit(1)
	}
	conf, _, _ := config.GetConfig(nil, "")
	if err = conf.Validate(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	var b []byte

	ext := filepath.Ext(configFile)
	if len(ext) > 1 {
		ext = ext[1:]
	}

	supportedExtensions := []string{"json", "toml", "yaml", "yml"}

	jsonBytes, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	jsonMap := make(map[string]interface{})
	err = json.Unmarshal(jsonBytes, &jsonMap)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	switch ext {
	case "json":
		b = jsonBytes
	case "toml":
		b, err = toml.Marshal(conf)
	case "yaml", "yml":
		b, err = yaml.Marshal(conf)
	default:
		err = errors.New("output config file must have one of supported extensions: " + strings.Join(supportedExtensions, ", "))
	}

	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile(configFile, b, 0644)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
