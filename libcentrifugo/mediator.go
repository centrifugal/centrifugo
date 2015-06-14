package libcentrifugo

// Mediator is an interface to work with libcentrifugo events from
// Go code. Implemented Mediator must be set to Application via
// corresponding Application method SetMediator.
type Mediator interface {
	Connect(pk ProjectKey, info ClientInfo) bool
	Subscribe(pk ProjectKey, ch Channel, info ClientInfo) bool
	Unsubscribe(pk ProjectKey, ch Channel, info ClientInfo)
	Disconnect(pk ProjectKey, info ClientInfo)
	Message(pk ProjectKey, ch Channel, data []byte, client ConnID, info *ClientInfo, fromClient bool) bool
}
