package centrifuge

import "context"

// Credentials allows to authenticate connection when set into context.
type Credentials struct {
	UserID   string
	ExpireAt int64
	Info     []byte
}

// credentialsContextKeyType is special type to safely use
// context for setting and getting Credentials.
type credentialsContextKeyType int

// CredentialsContextKey allows Go code to set Credentials into context.
var credentialsContextKey credentialsContextKeyType

// SetCredentials allows to set connection Credentials to context.
func SetCredentials(ctx context.Context, creds *Credentials) context.Context {
	ctx = context.WithValue(ctx, credentialsContextKey, creds)
	return ctx
}
