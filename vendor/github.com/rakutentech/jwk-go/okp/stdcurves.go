package okp

import (
	"io"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/box"
)

// Ed25519 represents an Ed25519 Key Pair
type Ed25519 struct{ OctetKeyPairBase }

// NewEd25519 Creates a new Ed25519 CurveOctetKeyPair
func NewEd25519(pub, priv []byte) Ed25519 {
	return Ed25519{OctetKeyPairBase{pub, priv}}
}

// ValidateEd25519 validates that a Ed25519 private/public key pair matches
func ValidateEd25519(pub, priv []byte) error {
	return validateEd25519(OctetKeyPairBase{pub, priv})
}

// Curve is the name of the elliptic curve
func (c Ed25519) Curve() string { return "Ed25519" }

// Algorithm is the algorithm used in conjunction with the curve
func (c Ed25519) Algorithm() string { return "EdDSA" }

// Ed448 represents an Ed448 Key Pair
type Ed448 struct{ OctetKeyPairBase }

// NewEd448 Creates a new Ed448 CurveOctetKeyPair
func NewEd448(pub, priv []byte) Ed448 {
	return Ed448{OctetKeyPairBase{pub, priv}}
}

// Curve is the name of the elliptic curve
func (c Ed448) Curve() string { return "Ed448" }

// Algorithm is the algorithm used in conjunction with the curve
func (c Ed448) Algorithm() string { return "EdDSA" }

// Curve25519 represents a Curve25519 Key Pair
type Curve25519 struct{ OctetKeyPairBase }

// NewCurve25519 Creates a new Curve25519 CurveOctetKeyPair
func NewCurve25519(pub, priv []byte) Curve25519 {
	return Curve25519{OctetKeyPairBase{pub, priv}}
}

// ValidateCurve25519 validates that a Curve25519 private/public key pair matches
func ValidateCurve25519(pub, priv []byte) error {
	return validateCurve25519(OctetKeyPairBase{pub, priv})
}

// Curve is the name of the elliptic curve
func (c Curve25519) Curve() string { return "X25519" }

// Algorithm is the algorithm used in conjunction with the curve
func (c Curve25519) Algorithm() string { return "ECDH-ES" }

// Curve448 represents a Curve448 Key Pair
type Curve448 struct{ OctetKeyPairBase }

// NewCurve448 Creates a new Curve448 CurveOctetKeyPair
func NewCurve448(pub, priv []byte) Curve448 {
	return Curve448{OctetKeyPairBase{pub, priv}}
}

// Curve is the name of the elliptic curve
func (c Curve448) Curve() string { return "X448" }

// Algorithm is the algorithm used in conjunction with the curve
func (c Curve448) Algorithm() string { return "ECDH-ES" }

// GenerateEd25519 Generates a new Ed25519 CurveOctetKeyPair
func GenerateEd25519(rand io.Reader) (Ed25519, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand)
	if err != nil {
		return Ed25519{}, err
	}
	return NewEd25519(pubKey[:], privKey[:32]), nil
}

// GenerateCurve25519 Generates a new X25519 CurveOctetKeyPair
func GenerateCurve25519(rand io.Reader) (Curve25519, error) {
	pubKey, privKey, err := box.GenerateKey(rand)
	if err != nil {
		return Curve25519{}, err
	}
	return NewCurve25519(pubKey[:], privKey[:]), nil
}
