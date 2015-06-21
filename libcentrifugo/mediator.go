package libcentrifugo

// Mediator is an interface to work with libcentrifugo events from
// Go code. Implemented Mediator must be set to Application via
// corresponding Application method SetMediator.
type Mediator interface {
	Connect(pk ProjectKey, client ConnID, user UserID)
	Subscribe(pk ProjectKey, ch Channel, client ConnID, user UserID)
	Unsubscribe(pk ProjectKey, ch Channel, client ConnID, user UserID)
	Disconnect(pk ProjectKey, client ConnID, user UserID)
	Message(pk ProjectKey, ch Channel, data []byte, client ConnID, info *ClientInfo) bool
}
