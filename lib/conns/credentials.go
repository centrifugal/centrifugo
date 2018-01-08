package conns

const (
	// OptionHidden for hidden connections.
	OptionHidden string = "hidden"
)

// Credentials ...
type Credentials struct {
	UserID string
	Info   []byte
	Opts   string
}
