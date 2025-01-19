package app

import (
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/spf13/cobra"
)

func Centrifugo() *cobra.Command {
	var configFile string
	cmd := &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – scalable real-time messaging server in language-agnostic way",
		Run: func(cmd *cobra.Command, args []string) {
			Run(cmd, configFile)
		},
	}
	cmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	config.DefineFlags(cmd)
	return cmd
}
