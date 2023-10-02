package jwtverify

import (
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

type Verifier interface {
	VerifyConnectToken(token string, skipVerify bool) (ConnectToken, error)
	VerifySubscribeToken(token string, skipVerify bool) (SubscribeToken, error)
}

type ConnectToken struct {
	// UserID tells an ID of connecting user.
	UserID string
	// ExpireAt allows setting time in future when connection must be validated.
	// Validation can be server-side using On().Refresh callback or client-side
	// if On().Refresh not set.
	ExpireAt int64
	// Info contains additional information about connection. It will be
	// included into Join/Leave messages, into Alive information, also
	// info becomes a part of published message if it was published from
	// client directly. In some cases having additional info can be an
	// overhead – but you are simply free to not use it.
	Info []byte
	// Meta is custom data to append to a connection. Can be retrieved later
	// for connection inspection. Never sent in a client protocol and accessible
	// from a backend-side only.
	Meta json.RawMessage
	// Subs is a map of channels to subscribe server-side with options.
	Subs map[string]centrifuge.SubscribeOptions
}

type SubscribeToken struct {
	// UserID tells an ID of subscribing user.
	UserID string
	// Channel client wants to subscribe. Will be compared with channel in
	// subscribe command.
	Channel string
	// Client is a deprecated claim for compatibility with Centrifugo v3.
	Client string
	// Options for subscription.
	Options centrifuge.SubscribeOptions
}
