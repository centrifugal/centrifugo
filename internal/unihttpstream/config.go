package unihttpstream

import "github.com/centrifugal/centrifuge"

type Config struct {
	// ProtocolVersion used by default. If not set then we use centrifuge.ProtocolVersion1.
	ProtocolVersion centrifuge.ProtocolVersion
	// MaxRequestBodySize limits request body size.
	MaxRequestBodySize int
}
