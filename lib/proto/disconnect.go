package proto

var (
	// DisconnectReasonNormal ...
	DisconnectReasonNormal = "normal"
	// DisconnectReasonShutdown ...
	DisconnectReasonShutdown = "shutdown"
	// DisconnectReasonInvalidToken ...
	DisconnectReasonInvalidToken = "invalid token"
	// DisconnectReasonInvalidMessage ...
	DisconnectReasonInvalidMessage = "invalid message"
	// DisconnectReasonServerError ...
	DisconnectReasonServerError = "internal server error"
)

// Disconnect ...
type Disconnect struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
}

var (
	// DisconnectNormal ...
	DisconnectNormal = &Disconnect{
		Reason:    DisconnectReasonNormal,
		Reconnect: true,
	}
	// DisconnectShutdown ...
	DisconnectShutdown = &Disconnect{
		Reason:    DisconnectReasonShutdown,
		Reconnect: true,
	}
	// DisconnectInvalidToken ...
	DisconnectInvalidToken = &Disconnect{
		Reason:    DisconnectReasonInvalidToken,
		Reconnect: false,
	}
	// DisconnectInvalidMessage ...
	DisconnectInvalidMessage = &Disconnect{
		Reason:    DisconnectReasonInvalidMessage,
		Reconnect: false,
	}
	// DisconnectServerError ...
	DisconnectServerError = &Disconnect{
		Reason:    DisconnectReasonServerError,
		Reconnect: true,
	}
)
