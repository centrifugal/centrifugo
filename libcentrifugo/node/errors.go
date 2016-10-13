package node

import (
	"errors"
)

var (
	// ErrInvalidMessage means that you sent invalid message to Centrifugo.
	ErrInvalidMessage = errors.New("invalid message")
	// ErrMethodNotFound means that method sent in command does not exist.
	ErrMethodNotFound = errors.New("method not found")
	// ErrNamespaceNotFound means that namespace in channel name does not exist.
	ErrNamespaceNotFound = errors.New("namespace not found")
	// ErrInternalServerError means server error, if returned this is a signal that
	// something went wrong with Centrifugo itself.
	ErrInternalServerError = errors.New("internal server error")
	// ErrNotAvailable means that resource is not enabled.
	ErrNotAvailable = errors.New("not available")
)
