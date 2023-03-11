package unihttpstream

import "github.com/centrifugal/centrifuge"

type Config struct {
	// MaxRequestBodySize limits request body size.
	MaxRequestBodySize int

	centrifuge.PingPongConfig
}
