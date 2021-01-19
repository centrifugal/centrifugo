package okp

// CurveSpec exposes a named elliptic curve
type CurveSpec interface {
	// Curve is the name of the elliptic curve.
	// This should be the standard name used in JWA.
	// (See RFCs: 7518, 8037)
	Curve() string

	// Algorithm is the algorithm used in conjunction with the curve,
	// as specified in JWA.
	// (See RFCs: 7518, 8037)
	Algorithm() string
}

// OctetKeyPair specifies an octet key pair, as specified by RFC 8037.
type OctetKeyPair interface {
	// PrivateKey is the bytes specifying the private key.
	PrivateKey() []byte

	// PublicKey is the bytes specifying the public key
	PublicKey() []byte
}

// CurveOctetKeyPair is a combination of an OctetKeyPair and CurveSpec.
type CurveOctetKeyPair interface {
	OctetKeyPair
	CurveSpec
}

// OctetKeyPairBase is a common type for implementing OctetKeyPairs
type OctetKeyPairBase struct {
	publicKey  []byte
	privateKey []byte
}

// PrivateKey is the bytes specifying the private key.
func (okp OctetKeyPairBase) PrivateKey() []byte { return okp.privateKey }

// PublicKey is the bytes specifying the public key
func (okp OctetKeyPairBase) PublicKey() []byte { return okp.publicKey }
