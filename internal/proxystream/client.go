package proxystream

import (
	"context"

	"github.com/centrifugal/centrifuge"
)

type Client interface {
	ID() string
	UserID() string
	IsSubscribed(string) bool
	Context() context.Context
	Transport() centrifuge.TransportInfo
	Unsubscribe(ch string, unsubscribe ...centrifuge.Unsubscribe)
	WritePublication(channel string, publication *centrifuge.Publication, sp centrifuge.StreamPosition) error
}
