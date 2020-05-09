package centrifuge

type tokenVerifier interface {
	VerifyConnectToken(token string) (connectToken, error)
	VerifySubscribeToken(token string) (subscribeToken, error)
	Reload(config Config) error
}

type connectToken struct {
	// UserID tells library an ID of connecting user.
	UserID string
	// ExpireAt allows to set time in future when connection must be validated.
	// Validation can be server-side using On().Refresh callback or client-side
	// if On().Refresh not set.
	ExpireAt int64
	// Info contains additional information about connection. It will be
	// included into Join/Leave messages, into Presence information, also
	// info becomes a part of published message if it was published from
	// client directly. In some cases having additional info can be an
	// overhead – but you are simply free to not use it.
	Info []byte
	// Channels slice contains channels to subscribe connection to on server-side.
	Channels []string
}

type subscribeToken struct {
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
	// It will be included into Join/Leave messages, into Presence information,
	// also channel info becomes a part of published message if it was published
	// from subscribed client directly.
	Info []byte
	// ExpireTokenOnly used to indicate that library must only check token
	// expiration but not turn on Subscription expiration checks on server side.
	// This allows to implement one-time subcription tokens.
	ExpireTokenOnly bool
}
