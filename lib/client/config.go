package client

import (
	"github.com/centrifugal/centrifugo/lib/proto"
)

// options contains various connection specific options.
type options struct {
	// Hidden allows to hide connection from public visibility - i.e.
	// connection  will be hidden from other clients - no join/leave events
	// will be sent for it, connection will not be part of presence information.
	Hidden bool `json:"hidden,omitempty"`
}

// Config contains client connection specific configuration.
type Config struct {
	Encoding    proto.Encoding
	Credentials *Credentials
}

// Credentials ...
type Credentials struct {
	UserID  string
	Info    []byte
	Expires int64
	// TODO: ?
	options *options
}

// credentialsContextKeyType ...
type credentialsContextKeyType int

// CredentialsContextKey ...
var CredentialsContextKey credentialsContextKeyType
