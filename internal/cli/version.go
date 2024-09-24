package cli

import (
	"fmt"
	"runtime"

	"github.com/centrifugal/centrifugo/v5/internal/build"

	"github.com/spf13/cobra"
)

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Centrifugo version information",
		Long:  `Print the version information of Centrifugo`,
		Run: func(cmd *cobra.Command, args []string) {
			Version()
		},
	}
}

func Version() {
	fmt.Printf("Centrifugo v%s (Go version: %s)\n", build.Version, runtime.Version())
}
