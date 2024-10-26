package unisse

import (
	"github.com/centrifugal/centrifuge"
	"github.com/centrifugal/centrifugo/v5/internal/tools"
)

type Config struct {
	// MaxRequestBodySize for POST requests when used.
	MaxRequestBodySize      int
	ConnectCodeToHTTPStatus tools.ConnectCodeToHTTPStatus
	centrifuge.PingPongConfig
}
