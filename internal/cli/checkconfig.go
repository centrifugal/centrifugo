package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/centrifugal/centrifugo/v6/internal/config"

	"github.com/spf13/cobra"
)

func CheckConfig() *cobra.Command {
	var checkConfigFile string
	var checkConfigStrict bool
	var checkConfigCmd = &cobra.Command{
		Use:   "checkconfig",
		Short: "Check configuration file",
		Long:  `Check Centrifugo configuration file`,
		Run: func(cmd *cobra.Command, args []string) {
			checkConfig(cmd, checkConfigFile, checkConfigStrict)
		},
	}
	checkConfigCmd.Flags().StringVarP(&checkConfigFile, "config", "c", "config.json", "path to config file to check")
	checkConfigCmd.Flags().BoolVarP(&checkConfigStrict, "strict", "s", false, "strict check - fail on unknown fields")
	return checkConfigCmd
}

func checkConfig(cmd *cobra.Command, checkConfigFile string, strict bool) {
	cfg, cfgMeta, err := config.GetConfig(cmd, checkConfigFile)
	if err != nil {
		fmt.Printf("error getting config: %v\n", err)
		os.Exit(1)
	}
	if cfgMeta.FileNotFound {
		fmt.Println("config file not found")
		os.Exit(1)
	}
	err = cfg.Validate()
	if err != nil {
		fmt.Printf("error validating config: %s\n", err)
		os.Exit(1)
	}
	if strict && len(cfgMeta.UnknownKeys) > 0 {
		fmt.Printf("unknown keys in config: %v\n", strings.Join(cfgMeta.UnknownKeys, ", "))
		os.Exit(1)
	}
}
