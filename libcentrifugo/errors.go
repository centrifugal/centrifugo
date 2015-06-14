package libcentrifugo

import (
	"errors"
)

var (
	ErrInvalidMessage      = errors.New("invalid message")
	ErrInvalidToken        = errors.New("invalid token")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrMethodNotFound      = errors.New("method not found")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrProjectNotFound     = errors.New("project not found")
	ErrNamespaceNotFound   = errors.New("namespace not found")
	ErrInternalServerError = errors.New("internal server error")
	ErrLimitExceeded       = errors.New("limit exceeded")
	ErrNotAvailable        = errors.New("not available")
	ErrConnectionExpired   = errors.New("connection expired")
	ErrSendTimeout         = errors.New("send timeout")
	ErrClientClosed        = errors.New("client is closed")
	ErrRejected            = errors.New("command rejected")
)
