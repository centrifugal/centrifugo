package centrifuge

// Disconnect allows to configure how client will be disconnected from server.
type Disconnect struct {
	// Reason is a short description of disconnect.
	Reason string `json:"reason"`
	// Reconnect gives client an advice to reconnect after disconnect or not.
	Reconnect bool `json:"reconnect"`
}

// Some predefined disconnect structures. Though it's always
// possible to create Disconnect with any field values on the fly.
var (
	// DisconnectNormal is clean disconnect when client cleanly closes connection.
	DisconnectNormal = &Disconnect{
		Reason:    "",
		Reconnect: true,
	}
	// DisconnectShutdown sent when node is going to shut down.
	DisconnectShutdown = &Disconnect{
		Reason:    "shutdown",
		Reconnect: true,
	}
	// DisconnectInvalidSign sent when client came with wrong sign.
	DisconnectInvalidSign = &Disconnect{
		Reason:    "invalid sign",
		Reconnect: false,
	}
	// DisconnectBadRequest sent when client uses malformed protocol
	// frames or wrong order of commands.
	DisconnectBadRequest = &Disconnect{
		Reason:    "bad request",
		Reconnect: false,
	}
	// DisconnectServerError sent when internal error occurred on server.
	DisconnectServerError = &Disconnect{
		Reason:    "internal server error",
		Reconnect: true,
	}
	// DisconnectExpired sent when client connection expired.
	DisconnectExpired = &Disconnect{
		Reason:    "expired",
		Reconnect: true,
	}
	// DisconnectStale sent to close connection that did not become
	// authenticated in configured interval after dialing.
	DisconnectStale = &Disconnect{
		Reason:    "stale",
		Reconnect: false,
	}
	// DisconnectSlow sent when client can't read messages fast enough.
	DisconnectSlow = &Disconnect{
		Reason:    "slow",
		Reconnect: true,
	}
	// DisconnectWriteError sent when an error occurred while writing to
	// client connection.
	DisconnectWriteError = &Disconnect{
		Reason:    "write error",
		Reconnect: true,
	}
)
