package jwk

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"

	"crypto/elliptic"

	"github.com/rakutentech/jwk-go/jwktypes"
	"github.com/rakutentech/jwk-go/okp"
)

// MarshalPublicJSON serializes only the public fields of the KeySpec to JSON.
// For public keys this gives exactly the same result as MarshalJSON()
func (k *KeySpec) MarshalPublicJSON() ([]byte, error) {
	publicKey, err := k.PublicOnly()
	if err != nil {
		return nil, err
	}
	return publicKey.MarshalJSON()
}

// MarshalPublicJSON serializes only the public fields of the keys
// inside the KeySpecSet to JSON (as JWKS).
// For public keys this gives exactly the same result as MarshalJSON()
func (ks *KeySpecSet) MarshalPublicJSON() ([]byte, error) {
	publicKeys := make([]KeySpec, len(ks.Keys))
	for i, k := range ks.Keys {
		publicKey, err := k.PublicOnly()
		if err != nil {
			return nil, err
		}
		publicKeys[i] = *publicKey
	}
	publicKeySet := KeySpecSet{publicKeys}
	return json.Marshal(publicKeySet)
}

// MarshalJSON serializes the KeySpec to JSON.
func (k *KeySpec) MarshalJSON() ([]byte, error) {
	jwk, err := k.ToJWK()
	if err != nil {
		return nil, err
	}
	return json.Marshal(jwk)
}

// ToJWK converts a KeySpec into a JWK struct.
func (k *KeySpec) ToJWK() (*JWK, error) {
	jwk, err := convertToJWK(k.Key)
	if err != nil {
		return nil, err
	}
	jwk.Kid = k.KeyID
	jwk.Alg = k.Algorithm
	jwk.Use = k.Use
	return jwk, nil
}

func convertToJWK(keyInterface interface{}) (*JWK, error) {
	switch key := keyInterface.(type) {
	case []byte:
		return &JWK{
			Kty: "oct",
			K: keyBytesFrom(key),
		}, nil
	case *rsa.PublicKey:
		return fromRSAPublic(key)
	case *rsa.PrivateKey:
		return fromRSAPrivate(key)
	case *ecdsa.PublicKey:
		return fromECPublic(key)
	case *ecdsa.PrivateKey:
		return fromECPrivate(key)
	case okp.CurveOctetKeyPair:
		return fromOKP(key), nil
	default:
		return nil, errors.New("unsupported key type (cannot convert to JWK)")
	}
}

func fromOKP(kp okp.CurveOctetKeyPair) *JWK {
	var jwk JWK

	pubKey := kp.PublicKey()
	privKey := kp.PrivateKey()

	jwk.Kty = jwktypes.OKP
	jwk.X = &keyBytes{pubKey}
	if privKey != nil {
		jwk.D = &keyBytes{privKey}
	}
	jwk.Crv = kp.Curve()

	return &jwk
}

func fromRSAPublic(public *rsa.PublicKey) (*JWK, error) {
	n := public.N.Bytes()

	if n == nil {
		return nil, errors.New("invalid RSA public key")
	}

	return &JWK{
		Kty: jwktypes.RSA,
		N:   keyBytesFrom(n),
		E:   keyBytesFromUInt64(uint64(public.E)),
	}, nil
}

func fromRSAPrivate(private *rsa.PrivateKey) (*JWK, error) {
	jwk, err := fromRSAPublic(&private.PublicKey)
	if err != nil {
		return nil, err
	}

	if private.D == nil {
		return nil, errors.New("invalid RSA private key: missing field d")
	}

	if len(private.Primes) != 2 {
		return nil, errors.New("invalid RSA private key: must have exactly 2 primes")
	}

	jwk.D = keyBytesFrom(private.D.Bytes())
	jwk.P = keyBytesFrom(private.Primes[0].Bytes())
	jwk.Q = keyBytesFrom(private.Primes[1].Bytes())

	// JWK RSA representations should have the precomputed values 'dp', 'dq' and 'qi'
	private.Precompute()
	jwk.Dp = keyBytesFrom(private.Precomputed.Dp.Bytes())
	jwk.Dq = keyBytesFrom(private.Precomputed.Dq.Bytes())
	jwk.Qi = keyBytesFrom(private.Precomputed.Qinv.Bytes())
	return jwk, nil
}

func fromECPublicWithExtras(public *ecdsa.PublicKey) (*JWK, *elliptic.CurveParams, int, error) {
	params := public.Params()
	if public.X == nil || public.Y == nil || params == nil {
		return nil, nil, 0, errors.New("invalid EC public key")
	}

	byteSize := curveByteSize(params)

	x := public.X.Bytes()
	y := public.Y.Bytes()

	if len(x) > byteSize || len(y) > byteSize {
		return nil, nil, 0, fmt.Errorf("invalid field x/y byte size for curve %s", params.Name)
	}

	return &JWK{
		Kty: jwktypes.EC,
		Crv: params.Name,
		X:   keyBytesFrom(x).padTo(byteSize), // See RFC 7518 # 6.2.1.2
		Y:   keyBytesFrom(y).padTo(byteSize), // See RFC 7518 # 6.2.1.3
	}, params, byteSize, nil
}

func fromECPublic(public *ecdsa.PublicKey) (*JWK, error) {
	jwk, _, _, err := fromECPublicWithExtras(public)
	return jwk, err
}

func fromECPrivate(private *ecdsa.PrivateKey) (*JWK, error) {
	jwk, params, byteSize, err := fromECPublicWithExtras(&private.PublicKey)
	if err != nil {
		return nil, err
	}

	if private.D == nil {
		return nil, errors.New("invalid EC private key")
	}

	d := private.D.Bytes()

	if len(d) > byteSize {
		return nil, fmt.Errorf("invalid field d byte size for curve %s", params.Name)
	}

	jwk.D = keyBytesFrom(d).padTo(byteSize) // See RFC 7518 # 6.2.2.1

	return jwk, nil
}

func curveByteSize(params *elliptic.CurveParams) int {
	// Bits may not be divisible by 8 with curves like P-521
	// We need to round up
	bitSize := params.BitSize
	byteSize := bitSize / 8
	if bitSize%8 != 0 {
		byteSize++
	}
	return byteSize
}
