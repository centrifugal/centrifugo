package proto

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
	// ErrMethodNotFound means that method sent in command does not exist.
	ErrMethodNotFound = &Error{
		Code:    101,
		Message: "method not found",
	}
	// ErrNamespaceNotFound means that namespace in channel name does not exist.
	ErrNamespaceNotFound = &Error{
		Code:    102,
		Message: "namespace not found",
	}
	// ErrNotAvailable means that resource is not enabled.
	ErrNotAvailable = &Error{
		Code:    103,
		Message: "not available",
	}
	// ErrPermissionDenied means that access to resource not allowed.
	ErrPermissionDenied = &Error{
		Code:    104,
		Message: "permission denied",
	}
	// ErrAlreadySubscribed returned when client wants to subscribe on channel
	// it already subscribed to.
	ErrAlreadySubscribed = &Error{
		Code:    105,
		Message: "already subscribed",
	}
	// ErrLimitExceeded says that some sort of limit exceeded, server logs should
	// give more detailed information.
	ErrLimitExceeded = &Error{
		Code:    106,
		Message: "limit exceeded",
	}
	// ErrInvalidData says that Centrifugo can not parse received data
	// because it is malformed.
	ErrInvalidData = &Error{
		Code:    107,
		Message: "invalid data",
	}
	// ErrUnauthorized says that ...
	ErrUnauthorized = &Error{
		Code:    108,
		Message: "unauthorized",
	}
)
