package jwtverify

import (
	"encoding/json"

	"github.com/centrifugal/centrifuge"
)

type Verifier interface {
	VerifyConnectToken(token string) (ConnectToken, error)
	VerifySubscribeToken(token string) (SubscribeToken, error)
}

type ConnectToken struct {
	// UserID tells library an ID of connecting user.
	UserID string
	// ExpireAt allows to set time in future when connection must be validated.
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
	// Subs is a map of channels to subscribe server-side with options. This is a
	// more advanced version of Channels actually. If Subs map is not empty then we
	// don't look at Channels at all.
	Subs map[string]centrifuge.SubscribeOptions
}

type SubscribeToken struct {
	// Client is a unique client ID string set to each connection on server.
	// Will be compared with actual client ID.
	Client string
	// Channel client wants to subscribe. Will be compared with channel in
	// subscribe command.
	Channel string
	// Options for subscription.
	Options centrifuge.SubscribeOptions
}
