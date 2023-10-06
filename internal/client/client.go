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
	AcquireStorage() (map[string]any, func(map[string]any))
	Unsubscribe(ch string, unsubscribe ...centrifuge.Unsubscribe)
	WritePublication(channel string, publication *centrifuge.Publication, sp centrifuge.StreamPosition) error
}
