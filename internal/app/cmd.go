package app

import (
	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/spf13/cobra"
)

// Centrifugo returns the root command running the server with opts.
func Centrifugo(opts Options) *cobra.Command {
	var configFile string
	cmd := &cobra.Command{
		Use:   "",
		Short: "Centrifugo",
		Long:  "Centrifugo – scalable real-time messaging server in language-agnostic way",
		Run: func(cmd *cobra.Command, args []string) {
			Run(cmd, configFile, opts)
		},
	}
	cmd.Flags().StringVarP(&configFile, "config", "c", "config.json", "path to config file")
	cmd.Flags().Bool("watch-config-file", false, "restart server when config file changes (for development)")
	config.DefineFlags(cmd)
	return cmd
}
