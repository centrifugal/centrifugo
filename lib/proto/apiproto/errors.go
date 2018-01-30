package apiproto

import (
	"fmt"
)

func (e Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// Here we define errors that can be exposed in server and client API replies.
var (
	// ErrInternalServerError means server error, if returned this is a signal
	// that something went wrong with Centrifugo itself.
	ErrInternalServerError = &Error{
		Code:    100,
		Message: "internal server error",
	}
	// ErrNamespaceNotFound means that namespace in channel name does not exist.
	ErrNamespaceNotFound = &Error{
		Code:    102,
		Message: "namespace not found",
	}
	// ErrMethodNotFound means that method sent in command does not exist.
	ErrMethodNotFound = &Error{
		Code:    104,
		Message: "method not found",
	}
	// ErrLimitExceeded says that some sort of limit exceeded, server logs should
	// give more detailed information.
	ErrLimitExceeded = &Error{
		Code:    106,
		Message: "limit exceeded",
	}
	// ErrBadRequest says that Centrifugo can not parse received data
	// because it is malformed.
	ErrBadRequest = &Error{
		Code:    107,
		Message: "bad request",
	}
	// ErrNotAvailable means that resource is not enabled.
	ErrNotAvailable = &Error{
		Code:    108,
		Message: "not available",
	}
)
