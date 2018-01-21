package client

const (
	// OptionHidden for hidden connections.
	OptionHidden string = "hidden"
)

// Credentials ...
type Credentials struct {
	UserID  string
	Info    []byte
	Expires int64
	Opts    string
}

// credentialsContextKeyType ...
type credentialsContextKeyType int

// CredentialsContextKey ...
var CredentialsContextKey credentialsContextKeyType = 1
