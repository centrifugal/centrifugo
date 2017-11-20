package proto

// DisconnectReason ...
type DisconnectReason string

var (
	// DisconnectReasonShutdown ...
	DisconnectReasonShutdown DisconnectReason = "shutdown"
	// DisconnectReasonInvalidToken ...
	DisconnectReasonInvalidToken DisconnectReason = "invalid token"
	// DisconnectReasonInvalidMessage ...
	DisconnectReasonInvalidMessage DisconnectReason = "invalid message"
	// DisconnectReasonServerError ...
	DisconnectReasonServerError DisconnectReason = "internal server error"
)

// DisconnectAdvice ...
type DisconnectAdvice struct {
	Reason    DisconnectReason `json:"reason"`
	Reconnect bool             `json:"reconnect"`
}
