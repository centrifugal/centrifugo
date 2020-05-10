package jwt

// Error represents a JWT error.
type Error string

func (e Error) Error() string {
	return string(e)
}

var _ error = (Error)("")

// Build and parse errors.
const (
	// ErrInvalidKey indicates that key is not valid.
	ErrInvalidKey = Error("jwt: key is not valid")

	// ErrUnsupportedAlg indicates that given algorithm is not supported.
	ErrUnsupportedAlg = Error("jwt: algorithm is not supported")

	// ErrInvalidFormat indicates that token format is not valid.
	ErrInvalidFormat = Error("jwt: token format is not valid")

	// ErrAudienceInvalidFormat indicates that audience format is not valid.
	ErrAudienceInvalidFormat = Error("jwt: audience format is not valid")

	// ErrDateInvalidFormat indicates that date format is not valid.
	ErrDateInvalidFormat = Error("jwt: date is not valid")

	// ErrAlgorithmMismatch indicates that token is signed by another algorithm.
	ErrAlgorithmMismatch = Error("jwt: token is signed by another algorithm")

	// ErrInvalidSignature indicates that signature is not valid.
	ErrInvalidSignature = Error("jwt: signature is not valid")
)
