package client

import (
	"time"

	"github.com/centrifugal/centrifugo/lib/proto"
)

// Config contains client connection specific configuration.
type Config struct {
	Encoding        proto.Encoding
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
