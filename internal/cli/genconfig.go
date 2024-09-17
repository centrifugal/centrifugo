package cli

import (
	"fmt"
	"os"

	"github.com/centrifugal/centrifugo/v5/internal/config"
	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/spf13/cobra"
)

func GenConfig(cmd *cobra.Command, outputConfigFile string) {
	err := tools.GenerateConfig(outputConfigFile)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	cfg, _, err := config.GetConfig(cmd, outputConfigFile)
	if err != nil {
		_ = os.Remove(outputConfigFile)
		fmt.Printf("error getting config: %v\n", err)
		os.Exit(1)
	}
	err = cfg.Validate()
	if err != nil {
		_ = os.Remove(outputConfigFile)
		fmt.Printf("error validating config: %v\n", err)
		os.Exit(1)
	}
}
