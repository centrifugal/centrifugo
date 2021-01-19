package jwk

import (
	"encoding/json"

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/rakutentech/jwk-go/jwktypes"
	"github.com/rakutentech/jwk-go/okp"
)

// UnmarshalJSON deserializes a KeySpec from the given JSON.
func (k *KeySpec) UnmarshalJSON(data []byte) error {
	jwk := &JWK{}
	err := json.Unmarshal(data, jwk)
	if err != nil {
		return err
	}

	key, err := convertFromJwk(jwk)
	if err != nil {
		return err
	}

	k.Key = key
	k.KeyID = jwk.Kid
	k.Algorithm = jwk.Alg
	k.Use = jwk.Use

	return nil
}

// ParseKeySpec parses JWK fields into a key that can be used by Go crypto libraries.
// The following key type conversions are supported:
//
// kty = "oct" -> []byte
// kty = "OKP" -> okp.OctetKeyPair (Ed25519 or X25519 keys for x/crypto libraries)
// kty = "RSA" -> rsa.PrivateKey / rsa.PublicKey
// kty = "EC"  -> ecdsa.PrivateKey / ecdsa.PublicKey
func (jwk *JWK) ParseKeySpec() (*KeySpec, error) {
	k := KeySpec{}
	key, err := convertFromJwk(jwk)
	if err != nil {
		return nil, err
	}

	k.Key = key
	k.KeyID = jwk.Kid
	k.Algorithm = jwk.Alg
	k.Use = jwk.Use

	return &k, nil
}

// UnmarshalJSON reads a key from its JSON representation.
func convertFromJwk(jwk *JWK) (interface{}, error) {
	switch jwk.Kty {
	case jwktypes.OctetKey:
		return jwk.unmarshalOctets()
	case jwktypes.EC:
		return jwk.unmarshalEC()
	case jwktypes.RSA:
		return jwk.unmarshalRSA()
	case jwktypes.OKP:
		return jwk.unmarshalOKP()
	default:
		return nil, fmt.Errorf("unknown key type: %s", jwk.Kty)
	}
}

func (jwk *JWK) unmarshalOctets() ([]byte, error) {
	if jwk.K == nil {
		return nil, errors.New("missing field k for symmetric key")
	}
	return jwk.K.data, nil
}

func (jwk *JWK) unmarshalRSA() (interface{}, error) {
	if jwk.N == nil || jwk.E == nil {
		return nil, errors.New("missing field n/e for RSA key")
	}

	e := jwk.E.toBigInt()
	if e.Sign() <= 0 {
		return nil, errors.New("exponent must be positive")
	}
	ei := e.Uint64()
	if ei > math.MaxInt32 {
		return nil, errors.New("exponent is too big")
	}

	public := rsa.PublicKey{
		N: jwk.N.toBigInt(),
		E: int(ei),
	}

	// If d, q and p are not available, this is a public key
	if jwk.D == nil || jwk.P == nil || jwk.Q == nil {
		return &public, nil
	}
	// Otherwise this is a private key

	return &rsa.PrivateKey{
		PublicKey: public,
		D:         jwk.D.toBigInt(),
		Primes:    []*big.Int{jwk.P.toBigInt(), jwk.Q.toBigInt()},
	}, nil
}

func (jwk *JWK) unmarshalEC() (interface{}, error) {
	if jwk.X == nil || jwk.Y == nil {
		return nil, errors.New("missing field x/y for EC key")
	}

	if jwk.Crv == "" {
		return nil, errors.New("missing elliptic curve (crv field) for EC key")
	}

	curve, ok := ecdsaCurves[jwk.Crv]
	if !ok {
		return nil, fmt.Errorf("unknown elliptic curve %s", jwk.Crv)
	}

	byteSize := curveByteSize(curve.Params())
	if len(jwk.X.data) != byteSize || len(jwk.Y.data) != byteSize {
		return nil, fmt.Errorf("field x/y have wrong size: both should be %d bytes", byteSize)
	}

	public := ecdsa.PublicKey{
		Curve: curve,
		X:     jwk.X.toBigInt(),
		Y:     jwk.Y.toBigInt(),
	}

	if !curve.IsOnCurve(public.X, public.Y) {
		return nil, errors.New("coordinate (x,y) is not on the elliptic curve")
	}

	// If d is not available, this is a public key
	if jwk.D == nil {
		return &public, nil
	}
	// Otherwise this is a private key

	if len(jwk.D.data) != byteSize {
		return nil, fmt.Errorf("field d should be %d bytes", byteSize)
	}

	dx, dy := curve.ScalarBaseMult(jwk.D.data)
	if dx.Cmp(public.X) != 0 || dy.Cmp(public.Y) != 0 {
		return nil, fmt.Errorf("field d does not match fields x/y")
	}

	return &ecdsa.PrivateKey{
		PublicKey: public,
		D:         jwk.D.toBigInt(),
	}, nil
}

func (jwk *JWK) unmarshalOKP() (okp.CurveOctetKeyPair, error) {
	if jwk.X == nil {
		return nil, errors.New("public key field x not specified for OKP")
	}

	pubKey := jwk.X.data
	var privKey []byte
	if jwk.D != nil {
		privKey = jwk.D.data
	}

	return okp.NewCurveOKP(jwk.Crv, pubKey, privKey)
}

// This list includes all the supported Weierstrass curves
var ecdsaCurves = map[string]elliptic.Curve{
	"P-256": elliptic.P256(),
	"P-384": elliptic.P384(),
	"P-521": elliptic.P521(),
}
