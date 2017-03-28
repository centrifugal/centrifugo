package main

import (
	// Register builtin memory and redis engines.
	_ "github.com/centrifugal/centrifugo/libcentrifugo/engine/enginememory"
	_ "github.com/centrifugal/centrifugo/libcentrifugo/engine/engineredis"

	// Register servers.
	_ "github.com/centrifugal/centrifugo/libcentrifugo/server/httpserver"

	// Register embedded web interface.
	_ "github.com/centrifugal/centrifugo/libcentrifugo/statik"

	"github.com/centrifugal/centrifugo/libcentrifugo/centrifugo"
)

// VERSION of Centrifugo server. Set on build stage.
var VERSION string

func main() {
	centrifugo.Main(VERSION)
}
