package jwk

import (
	"crypto/ecdsa"
	"strings"

	"github.com/rakutentech/jwk-go/okp"
)

// IsKeyType returns true if KeySpec has expected key type and curve.
//
// If expectedKty contains a slash, then it will be parsed as a pair of key
// type ('kty') and curve ('crv'), in the following format: 'kty/crv', and
// the KeySpec will match if both its key type and curve match. Otherwise,
// only the key type will be checked.
func (k *KeySpec) IsKeyType(expectedKty string) bool {
	keyTypeParts := strings.SplitN(expectedKty, "/", 2)
	var expectedCurve string
	if len(keyTypeParts) == 2 {
		expectedCurve = keyTypeParts[1]
		expectedKty = keyTypeParts[0]
	}

	kty, curve, _ := k.KeyType()
	return kty == expectedKty && (expectedCurve == "" || curve == expectedCurve)
}

// Filter filters the specified KeySpecSet with a filter function
//
// filter is a predicate that accepts a KeySpec and returns a boolean value.
func (ks KeySpecSet) Filter(filter func(key *KeySpec) bool) KeySpecSet {
	var newKeys []KeySpec
	for _, key := range ks.Keys {
		if filter(&key) {
			newKeys = append(newKeys, key)
		}
	}
	return KeySpecSet{newKeys}
}

// OnlyKeyTypes filters the KeySpecSet and returns only KeySpecs which match
// the specified key type and curve.
//
// If keyType contains a slash, then it will be parsed as a pair of key type
// ('kty') and curve ('crv'), in the following format: 'kty/crv', and  the
// KeySpec will match if both its key type and curve match. Otherwise, only
// the key type will be checked.
func (ks KeySpecSet) OnlyKeyTypes(keyTypes ...string) KeySpecSet {
	return ks.Filter(func(key *KeySpec) bool {
		for _, allowedKty := range keyTypes {
			if key.IsKeyType(allowedKty) {
				return true
			}
		}
		return false
	})
}

// PrimaryKey returns the first KeySpec in the KeySpecSet which matches the
// specified key type and curve.
//
// If keyType contains a slash, then it will be parsed as a pair of key type
// ('kty') and curve ('crv'), in the following format: 'kty/crv', and  the
// KeySpec will match if both its key type and curve match. Otherwise, only
// the key type will be checked.
func (ks KeySpecSet) PrimaryKey(keyType string) *KeySpec {
	for _, k := range ks.Keys {
		if k.IsKeyType(keyType) {
			return &k
		}
	}
	return nil
}

// PrimaryCurveOKP returns the first CurveOctetKeyPair in the KeySpecSet
// which matches the specified curve name.
//
// If keyType contains a slash, then it will be parsed as a pair of key type
// ('kty') and curve ('crv'), in the following format: 'kty/crv', and  the
// KeySpec will match if both its key type and curve match. Otherwise, only
// the key type will be checked.
func (ks KeySpecSet) PrimaryCurveOKP(curve string) (*KeySpec, okp.CurveOctetKeyPair) {
	for _, k := range ks.Keys {
		curveOKP, ok := k.Key.(okp.CurveOctetKeyPair)
		if ok && curveOKP.Curve() == curve {
			return &k, curveOKP
		}
	}
	return nil, nil
}

// PrimaryECDSAPrivate returns the first ECDSA PrivateKey in the KeySpecSet
// which matches the specified curve name.
func (ks KeySpecSet) PrimaryECDSAPrivate() (*KeySpec, *ecdsa.PrivateKey) {
	for _, k := range ks.Keys {
		if ecdsaPrivateKey, ok := k.Key.(*ecdsa.PrivateKey); ok {
			return &k, ecdsaPrivateKey
		}
	}
	return nil, nil
}
