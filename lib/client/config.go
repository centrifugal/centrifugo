package client

import (
	"time"
)

// Config contains client connection specific configuration.
type Config struct {
	Credentials     *Credentials
	StaleCloseDelay time.Duration
}

// Credentials ...
type Credentials struct {
	UserID string
	Exp    int64
	Info   []byte
}

// credentialsContextKeyType ...
type credentialsContextKeyType int

// CredentialsContextKey ...
var CredentialsContextKey credentialsContextKeyType
