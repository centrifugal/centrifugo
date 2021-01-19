package jwk

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"strconv"

	"github.com/rakutentech/jwk-go/jwktypes"
	"github.com/rakutentech/jwk-go/okp"
)

// KeyType returns the key type based on the key.
func (k *KeySpec) KeyType() (kty string, curve string, private bool) {
	switch key := k.Key.(type) {
	case []byte:
		kty = jwktypes.OctetKey
		private = true
	case *rsa.PublicKey:
		kty = jwktypes.RSA
		curve = strconv.Itoa(key.N.BitLen())
		private = false
		return
	case *rsa.PrivateKey:
		kty = jwktypes.RSA
		curve = strconv.Itoa(key.N.BitLen())
		return
	case *ecdsa.PublicKey:
		kty = jwktypes.EC
		curve = key.Curve.Params().Name
		private = false
	case *ecdsa.PrivateKey:
		kty = jwktypes.EC
		curve = key.Curve.Params().Name
		private = true
	case okp.CurveOctetKeyPair:
		kty = jwktypes.OKP
		curve = key.Curve()
		private = key.PrivateKey() != nil
	}
	return
}
