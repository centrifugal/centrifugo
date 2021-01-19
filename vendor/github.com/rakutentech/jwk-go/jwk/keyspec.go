package jwk

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json"
	"errors"

	"github.com/rakutentech/jwk-go/okp"
)

// KeySpec contains a cryptographic key accompanied by JWK metadata.
// This structure is used to contain information from parsed JSON Web Keys.
type KeySpec struct {
	Key       interface{}
	KeyID     string
	Algorithm string
	Use       string
}

// KeySpecSet represents a set of parsed JSON Web Keys
type KeySpecSet struct {
	Keys []KeySpec `json:"keys"`
}

// NewSpec directly creates a KeySpec from a concrete key object.
//
// Key object types supported:
// rsa.PrivateKey, rsa.PublicKey, ecdsa.PrivateKey, ecdsa.PublicKey,
// okp.OctetKeyPair, []byte
func NewSpec(key interface{}) *KeySpec {
	return &KeySpec{Key: key}
}

// NewSpecWithID directly creates a KeySpec from a concrete key object and a Key ID.
//
// Key object types supported:
// rsa.PrivateKey, rsa.PublicKey, ecdsa.PrivateKey, ecdsa.PublicKey,
// okp.OctetKeyPair, []byte
func NewSpecWithID(kid string, key interface{}) *KeySpec {
	return &KeySpec{Key: key, KeyID: kid}
}

// Parse parses a JWK string into a KeySpec
func Parse(jwkStr string) (*KeySpec, error) {
	return ParseBytes([]byte(jwkStr))
}

// ParseBytes parses JWK bytes into a KeySpec
func ParseBytes(data []byte) (*KeySpec, error) {
	k := &KeySpec{}
	err := json.Unmarshal(data, k)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// MustParse parses a JWK string into a KeySpec, panicking on error
func MustParse(jwkStr string) *KeySpec {
	return MustParseBytes([]byte(jwkStr))
}

// MustParseBytes parses JWK bytes into a KeySpec, panicking on error
func MustParseBytes(data []byte) *KeySpec {
	k, err := ParseBytes(data)
	if err != nil {
		panic(err)
	}
	return k
}

// IsPublic checks whether the key in KeySpec is a public key or not.
// In case the key is symmetric, this function returns false.
func (k *KeySpec) IsPublic() bool {
	switch key := k.Key.(type) {
	case okp.CurveOctetKeyPair:
		return key.PrivateKey() == nil
	case *ecdsa.PublicKey, *rsa.PublicKey:
		return true
	default:
		return false
	}
}

// PublicOnly removes all private components from a KeySpec (if they exists) and returns
// a key which can be safely published as a public key.
func (k *KeySpec) PublicOnly() (*KeySpec, error) {
	var pubKey crypto.PublicKey
	var err error
	switch key := k.Key.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
		return k, nil // Already public-only
	case *rsa.PrivateKey:
		pubKey = key.Public()
	case *ecdsa.PrivateKey:
		pubKey = key.Public()
	case okp.CurveOctetKeyPair:
		if key.PrivateKey() == nil {
			return k, nil // Already public-only
		}
		pubKey, err = okp.NewCurveOKP(key.Curve(), key.PublicKey(), nil)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("key type does not support extracting the public key")
	}
	return &KeySpec{
		Key:       pubKey,
		Algorithm: k.Algorithm,
		KeyID:     k.KeyID,
		Use:       k.Use,
	}, nil
}

// Clone creates a copy of a KeySpec
func (k *KeySpec) Clone() *KeySpec {
	return &KeySpec{
		Key:       k.Key,
		Algorithm: k.Algorithm,
		KeyID:     k.KeyID,
		Use:       k.Use,
	}
}
