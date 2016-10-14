package node

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/proto"
)

// Mediator is an interface to work with Centrifugo events from
// Go code. Implemented Mediator must be set to Application via
// corresponding Application method SetMediator.
type Mediator interface {
	Connect(client proto.ConnID, user proto.UserID) bool
	Subscribe(ch proto.Channel, client proto.ConnID, user proto.UserID) bool
	Unsubscribe(ch proto.Channel, client proto.ConnID, user proto.UserID)
	Disconnect(client proto.ConnID, user proto.UserID)
	Message(ch proto.Channel, data []byte, client proto.ConnID, info *proto.ClientInfo) bool
}
