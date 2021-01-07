package jwtverify

type Verifier interface {
	VerifyConnectToken(token string) (ConnectToken, error)
	VerifySubscribeToken(token string) (SubscribeToken, error)
}

type JWK struct {
	// KeyType The parameter identifies the cryptographic algorithm
	// family used with the key, such as "RSA" or "EC".
	// See https://tools.ietf.org/html/rfc7517#section-4.1
	KeyType string `json:"kty"`

	// Use parameter identifies the intended use of the public key. Available values: "sig" and "enc".
	// See https://tools.ietf.org/html/rfc7517#section-4.2
	Use string `json:"use"`

	// Algorithm parameter identifies the algorithm intended for use with the key.
	// See https://tools.ietf.org/html/rfc7517#section-4.4
	Algorithm string `json:"alg"`

	// KeyID parameter is used to match a specific key.
	// This is used, for instance, to choose among a set of keys within a JWK Set during key rollover.
	// See https://tools.ietf.org/html/rfc7517#section-4.5
	KeyID string `json:"kid"`

	N string `json:"n"`
	E string `json:"e"`
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
	// Channels slice contains channels to subscribe connection to on server-side.
	Channels []string
}

type SubscribeToken struct {
	// Client is a unique client ID string set to each connection on server.
	// Will be compared with actual client ID.
	Client string
	// Channel client wants to subscribe. Will be compared with channel in
	// subscribe command.
	Channel string
	// ExpireAt allows to set time in future when connection must be validated.
	// Validation can be server-side using On().SubRefresh callback or client-side
	// if On().SubRefresh not set.
	ExpireAt int64
	// Info contains additional information about connection in channel.
	// It will be included into Join/Leave messages, into Alive information,
	// also channel info becomes a part of published message if it was published
	// from subscribed client directly.
	Info []byte
	// ExpireTokenOnly used to indicate that library must only check token
	// expiration but not turn on Subscription expiration checks on server side.
	// This allows to implement one-time subscription tokens.
	ExpireTokenOnly bool
}
