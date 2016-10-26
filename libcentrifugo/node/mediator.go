package node

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

// Mediator is an interface to work with Centrifugo events from
// Go code. Implemented Mediator must be set to Application via
// corresponding Application method SetMediator.
type Mediator interface {
	Connect(client string, user string) bool
	Subscribe(ch string, client string, user string) bool
	Unsubscribe(ch string, client string, user string)
	Disconnect(client string, user string)
	Message(ch string, data []byte, client string, info *proto.ClientInfo) bool
}
