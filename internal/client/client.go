package client

import (
	"context"

	"github.com/centrifugal/centrifuge"
)

// Client represents client connection.
type Client interface {
	ID() string
	UserID() string
	IsSubscribed(string) bool
	Context() context.Context
	Transport() centrifuge.TransportInfo
}
