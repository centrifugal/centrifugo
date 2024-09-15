package cli

import (
	"fmt"
	"runtime"

	"github.com/centrifugal/centrifugo/v5/internal/build"
)

func Version() {
	fmt.Printf("Centrifugo v%s (Go version: %s)\n", build.Version, runtime.Version())
}
