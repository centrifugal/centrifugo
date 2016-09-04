package libcentrifugo

import (
	"github.com/centrifugal/centrifugo/libcentrifugo/message"
)

// Mediator is an interface to work with libcentrifugo events from
// Go code. Implemented Mediator must be set to Application via
// corresponding Application method SetMediator.
type Mediator interface {
	Connect(client message.ConnID, user message.UserID)
	Subscribe(ch message.Channel, client message.ConnID, user message.UserID)
	Unsubscribe(ch message.Channel, client message.ConnID, user message.UserID)
	Disconnect(client message.ConnID, user message.UserID)
	Message(ch message.Channel, data []byte, client message.ConnID, info *message.ClientInfo) bool
}
