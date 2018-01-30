package events

// Mediator allows to proxy Centrifugo events to Go application code.
type Mediator struct {
	// ConnectHandler reacts on connect events.
	ConnectHandler ConnectHandler
	// DisconnectHandler reacts on disconnect events.
	DisconnectHandler DisconnectHandler
	// SubscribeHandler reacts on subscribe events.
	SubscribeHandler SubscribeHandler
	// UnsubscribeHandler reacts on unsubscribe events.
	UnsubscribeHandler UnsubscribeHandler
	// PublishHandler reacts on publish requests.
	PublishHandler PublishHandler
	// PresenceHandler allows to register action to be executed on every
	// periodic connection presence update.
	PresenceHandler PresenceHandler
	// RPCHandler allows to register custom logic on incoming RPC calls.
	RPCHandler RPCHandler
}
