package centrifuge

import "context"

// Credentials allows to authenticate connection when set into context.
type Credentials struct {
	// UserID tells library an ID of connecting user.
	UserID string
	// ExpireAt allows to set time in future when connection must be validated.
	// In this case OnRefresh callback must be set by application.
	ExpireAt int64
	// Info contains additional information about connection. It will be
	// included into Join/Leave messages, into Presense information, also
	// info becomes a part of published message if it was published from
	// client directly. In some cases having additional info can be an
	// overhead – but you are simply free to not use it.
	Info []byte
}

// credentialsContextKeyType is special type to safely use
// context for setting and getting Credentials.
type credentialsContextKeyType int

// CredentialsContextKey allows Go code to set Credentials into context.
var credentialsContextKey credentialsContextKeyType

// SetCredentials allows to set connection Credentials to context. Credentias set
// to context will be used by centrifuge library then to authenticate user.
func SetCredentials(ctx context.Context, creds *Credentials) context.Context {
	ctx = context.WithValue(ctx, credentialsContextKey, creds)
	return ctx
}
