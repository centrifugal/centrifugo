package cli

import (
	"fmt"
	"runtime"

	"github.com/centrifugal/centrifugo/v6/internal/build"

	"github.com/spf13/cobra"
)

func Version() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version information",
		Long:  `Print the version information of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			version()
		},
	}
}

func version() {
	fmt.Printf("Centrifugo v%s (Go version: %s)\n", build.Version, runtime.Version())
}
