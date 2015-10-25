package libcentrifugo

// Mediator is an interface to work with libcentrifugo events from
// Go code. Implemented Mediator must be set to Application via
// corresponding Application method SetMediator.
type Mediator interface {
	Connect(client ConnID, user UserID)
	Subscribe(ch Channel, client ConnID, user UserID)
	Unsubscribe(ch Channel, client ConnID, user UserID)
	Disconnect(client ConnID, user UserID)
	Message(ch Channel, data []byte, client ConnID, info *ClientInfo) bool
}
