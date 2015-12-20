package libcentrifugo

import (
	"errors"
)

var (
	// ErrInvalidMessage means that you sent invalid message to Centrifugo.
	ErrInvalidMessage = errors.New("invalid message")
	// ErrInvalidToken means that client sent invalid token.
	ErrInvalidToken = errors.New("invalid token")
	// ErrUnauthorized means unauthorized access.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrMethodNotFound means that method sent in command does not exist.
	ErrMethodNotFound = errors.New("method not found")
	// ErrPermissionDenied means that access to resource not allowed.
	ErrPermissionDenied = errors.New("permission denied")
	// ErrNamespaceNotFound means that namespace in channel name does not exist.
	ErrNamespaceNotFound = errors.New("namespace not found")
	// ErrInternalServerError means server error, if returned this is a signal that
	// something went wrong with Centrifugo itself.
	ErrInternalServerError = errors.New("internal server error")
	// ErrLimitExceeded says that some sort of limit exceeded, server logs should give
	// more detailed information.
	ErrLimitExceeded = errors.New("limit exceeded")
	// ErrNotAvailable means that resource is not enabled.
	ErrNotAvailable = errors.New("not available")
	// ErrSendTimeout means that timeout occurred when sending message into connection.
	ErrSendTimeout = errors.New("send timeout")
	// ErrClientClosed means that client connection already closed.
	ErrClientClosed = errors.New("client is closed")
)
