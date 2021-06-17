package apiproto

import (
	"fmt"
)

func (x *Error) Error() string {
	return fmt.Sprintf("%d: %s", x.Code, x.Message)
}

// Here we define errors that can be exposed in server API replies.
var (
	// ErrorInternal means server error, if returned this is a signal
	// that something went wrong with Centrifugo itself.
	ErrorInternal = &Error{
		Code:    100,
		Message: "internal server error",
	}
	// ErrorNamespaceNotFound means that namespace in channel name does not exist.
	ErrorNamespaceNotFound = &Error{
		Code:    102,
		Message: "namespace not found",
	}
	// ErrorMethodNotFound means that method sent in command does not exist.
	ErrorMethodNotFound = &Error{
		Code:    104,
		Message: "method not found",
	}
	// ErrorLimitExceeded says that some sort of limit exceeded, server logs should
	// give more detailed information.
	ErrorLimitExceeded = &Error{
		Code:    106,
		Message: "limit exceeded",
	}
	// ErrorBadRequest says that Centrifugo can not parse received data
	// because it is malformed.
	ErrorBadRequest = &Error{
		Code:    107,
		Message: "bad request",
	}
	// ErrorNotAvailable means that resource is not enabled.
	ErrorNotAvailable = &Error{
		Code:    108,
		Message: "not available",
	}
	// ErrorUnrecoverablePosition means that stream does not contain required
	// range of publications to fulfill a history query. This can be happen to
	// expiration, size limitation or due to wrong epoch.
	ErrorUnrecoverablePosition = &Error{
		Code:    112,
		Message: "unrecoverable position",
	}
)
