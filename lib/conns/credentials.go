package conns

// CredentialsKeyType ...
type CredentialsKeyType int

// CredentialsKey ...
var CredentialsKey CredentialsKeyType = 1

const (
	// FlagPassive for passive connections.
	FlagPassive int = 1 << iota
)

// Credentials ...
type Credentials struct {
	UserID string
	Info   []byte
	Flags  int
}
