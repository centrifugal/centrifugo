package cli

import (
	"fmt"
	"os"

	"github.com/centrifugal/centrifugo/v5/internal/config"

	"github.com/spf13/cobra"
)

func CheckConfig(cmd *cobra.Command, checkConfigFile string) {
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
}
