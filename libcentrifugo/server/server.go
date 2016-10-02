package server

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/engine"
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

type Server interface {
	// Run runs the server once node ready.
	Run() error

	// Shutdown shuts down the server.
	Shutdown() error
}
