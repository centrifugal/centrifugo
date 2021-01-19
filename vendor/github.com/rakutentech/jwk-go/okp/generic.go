package okp

import (
	"errors"
	"fmt"
)

var (
	// ErrKeyMissing s thrown when a required key (public or private) is missing
	ErrKeyMissing = errors.New("no public or private key is specified")
)

// NewCurveOKP creates a new CurveOctetKeyPair with the specified curve
func NewCurveOKP(curve string, pubKey []byte, privKey []byte) (CurveOctetKeyPair, error) {
	okpb := OctetKeyPairBase{pubKey, privKey}
	switch curve {
	case "Ed25519":
		if err := validateEd25519(okpb); err != nil {
			return nil, err
		}
		return Ed25519{okpb}, nil
	case "Ed448":
		// TODO: Ed448 is not validated because we don't really support it yet
		return Ed448{okpb}, nil
	case "X25519":
		if err := validateCurve25519(okpb); err != nil {
			return nil, err
		}
		return Curve25519{okpb}, nil
	case "X448":
		// TODO: Curve448 is not validated because we don't really support it yet
		return Curve448{okpb}, nil
	default:
		return nil, fmt.Errorf("unknown curve specified: %s", curve)
	}
}

// CurveExtractPublic creates a new CurveOctetKeyPair out of an existing one,
// removing the private key and keeping only the public key.
func CurveExtractPublic(okp CurveOctetKeyPair) (CurveOctetKeyPair, error) {
	return NewCurveOKP(okp.Curve(), okp.PublicKey(), nil)
}
