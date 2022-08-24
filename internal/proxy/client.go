package proxy

import (
	"context"

	"github.com/centrifugal/centrifuge"
)

type Client interface {
	ID() string
	UserID() string
	Context() context.Context
	Transport() centrifuge.TransportInfo
}
