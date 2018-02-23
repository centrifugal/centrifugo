package centrifuge

// Disconnect allows to configure how client will be disconnected from server.
type Disconnect struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
}

// Some predefined disconnect structures. Though it's always
// possible to create Disconnect with any field values on the fly.
var (
	// DisconnectNormal ...
	DisconnectNormal = &Disconnect{
		Reason:    "",
		Reconnect: true,
	}
	// DisconnectShutdown ...
	DisconnectShutdown = &Disconnect{
		Reason:    "shutdown",
		Reconnect: true,
	}
	// DisconnectInvalidSign ...
	DisconnectInvalidSign = &Disconnect{
		Reason:    "invalid sign",
		Reconnect: false,
	}
	// DisconnectBadRequest ...
	DisconnectBadRequest = &Disconnect{
		Reason:    "bad request",
		Reconnect: false,
	}
	// DisconnectServerError ...
	DisconnectServerError = &Disconnect{
		Reason:    "internal server error",
		Reconnect: true,
	}
)
