package unisse

import "github.com/centrifugal/centrifuge"

type Config struct {
	// MaxRequestBodySize for POST requests when used.
	MaxRequestBodySize int

	centrifuge.PingPongConfig
}
