package cli

import (
	"fmt"
	"os"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/spf13/cobra"
)

func GenConfig() *cobra.Command {
	var outputConfigFile string
	var genConfigCmd = &cobra.Command{
		Use:   "genconfig",
		Short: "Generate minimal configuration file to start with",
		Long:  `Generate minimal configuration file to start with`,
		Run: func(cmd *cobra.Command, args []string) {
			genConfig(cmd, outputConfigFile)
		},
	}
	genConfigCmd.Flags().StringVarP(&outputConfigFile, "config", "c", "config.json", "path to output config file")
	return genConfigCmd
}

func genConfig(cmd *cobra.Command, outputConfigFile string) {
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
