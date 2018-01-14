package proto

var (
	// DisconnectReasonNormal ...
	DisconnectReasonNormal = "normal"
	// DisconnectReasonShutdown ...
	DisconnectReasonShutdown = "shutdown"
	// DisconnectReasonInvalidSign ...
	DisconnectReasonInvalidSign = "invalid sign"
	// DisconnectReasonBadRequest ...
	DisconnectReasonBadRequest = "bad request"
	// DisconnectReasonServerError ...
	DisconnectReasonServerError = "internal server error"
)

// Disconnect ...
type Disconnect struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
}

// Some predefined disconnect structures. Though it's always
// possible to create Disconnect with any field values on the fly.
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
	// DisconnectInvalidSign ...
	DisconnectInvalidSign = &Disconnect{
		Reason:    DisconnectReasonInvalidSign,
		Reconnect: false,
	}
	// DisconnectBadRequest ...
	DisconnectBadRequest = &Disconnect{
		Reason:    DisconnectReasonBadRequest,
		Reconnect: false,
	}
	// DisconnectServerError ...
	DisconnectServerError = &Disconnect{
		Reason:    DisconnectReasonServerError,
		Reconnect: true,
	}
)
