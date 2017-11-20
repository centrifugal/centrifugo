package proto

import (
	"errors"
)

var (
	// ErrInternalServerError means server error, if returned this is a signal
	// that something went wrong with Centrifugo itself.
	ErrInternalServerError = errors.New("internal server error")
	// ErrMethodNotFound means that method sent in command does not exist.
	ErrMethodNotFound = errors.New("method not found")
	// ErrNamespaceNotFound means that namespace in channel name does not exist.
	ErrNamespaceNotFound = errors.New("namespace not found")
	// ErrNotAvailable means that resource is not enabled.
	ErrNotAvailable = errors.New("not available")
	// ErrPermissionDenied means that access to resource not allowed.
	ErrPermissionDenied = errors.New("permission denied")
	// ErrAlreadySubscribed returned when client wants to subscribe on channel
	// it already subscribed to.
	ErrAlreadySubscribed = errors.New("already subscribed")
	// ErrLimitExceeded says that some sort of limit exceeded, server logs should
	// give more detailed information.
	ErrLimitExceeded = errors.New("limit exceeded")
)
