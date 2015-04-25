package main

import (
	"errors"
)

var (
	ErrInvalidClientMessage  = errors.New("invalid client message")
	ErrInvalidApiMessage     = errors.New("invalid API message")
	ErrInvalidControlMessage = errors.New("invalid control message")
	ErrInvalidAdminMessage   = errors.New("invalid admin message")
	ErrInvalidToken          = errors.New("invalid token")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrMethodNotFound        = errors.New("method not found")
	ErrPermissionDenied      = errors.New("permission denied")
	ErrProjectNotFound       = errors.New("project not found")
	ErrNamespaceNotFound     = errors.New("namespace not found")
	ErrInternalServerError   = errors.New("internal server error")
	ErrLimitExceeded         = errors.New("limit exceeded")
	ErrNotAvailable          = errors.New("not available")
	ErrConnectionExpired     = errors.New("connection expired")
)
