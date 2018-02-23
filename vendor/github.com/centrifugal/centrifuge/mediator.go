package centrifuge

// Mediator allows to proxy Centrifugo events to Go application code.
type Mediator struct {
	// Connect called every time client connects to node.
	Connect ConnectHandler
	// Disconnect called when client disconnected.
	Disconnect DisconnectHandler
	// Subscribe called when client subscribes on channel.
	Subscribe SubscribeHandler
	// Unsubscribe called when client unsubscribes from channel.
	Unsubscribe UnsubscribeHandler
	// Publish called when client publishes message into channel.
	Publish PublishHandler
	// Presence allows to register action to be executed on every
	// periodic client connection presence update.
	Presence PresenceHandler
	// Refresh called when it's time to refresh connection credentials.
	Refresh RefreshHandler
	// RPC allows to register custom logic on incoming RPC calls.
	RPC RPCHandler
	// Message called when client sent asynchronous message.
	Message MessageHandler
}
