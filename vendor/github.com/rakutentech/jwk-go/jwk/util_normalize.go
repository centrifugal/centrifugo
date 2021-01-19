package jwk

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"

	"hash"

	"github.com/rakutentech/jwk-go/okp"
)

func getKeyAlgo(key interface{}, sig bool) string {
	switch k := key.(type) {
	case *ecdsa.PrivateKey, *ecdsa.PublicKey:
		if sig {
			return DefaultECSignAlg
		} else if UseKeyWrapForECDH {
			return DefaultECKeyAlgWithKeyWrap
		} else {
			return DefaultECKeyAlg
		}
	case *rsa.PrivateKey, *rsa.PublicKey:
		if sig {
			return DefaultRSASignAlg
		}
		return DefaultRSAKeyAlg
	case okp.CurveOctetKeyPair:
		return k.Curve()
	case []byte:
		if sig {
			return DefaultHMACSignAlg
		}
		return DefaultAESKeyAlg
	default:
		return ""
	}
}

// NormalizationSettings contains settings for the normalization preformed by
// KeySpec.Normalize()
type NormalizationSettings struct {
	// Use tells KeySpec.Normalize() to set the 'use' field to the specified string
	Use string

	// RequireKeyID tells KeySpec.Normalize() to attempt to auto-generate 'kid' if it
	// is missing and fail if it couldn't generate it automatically.
	// Automatic generation is done by creating a SHA-256 thumbprint of the key.
	RequireKeyID bool

	// RequireAlgorithm requires 'alg' to be specified.
	RequireAlgorithm bool

	// ThumbprintHashFunc specifies the hash function to use for the thumbprint
	ThumbprintHashFunc hash.Hash
}

// Normalize attempts to put some uniformity on the metadata fields attached to the JSON Web Key
// The normalization is influenced by the settings parameter.
func (k *KeySpec) Normalize(settings NormalizationSettings) error {
	// Normalize key use
	if k.Use == "" {
		k.Use = settings.Use
	} else if settings.Use != "" {
		if k.Use != settings.Use {
			return fmt.Errorf("expected key use to be '%s' but got '%s", settings.Use, k.Use)
		}
	}

	// Write algorithm only if empty
	if k.Algorithm == "" {
		switch k.Use {
		case "sig":
			k.Algorithm = getKeyAlgo(k.Key, true)
		case "enc":
			k.Algorithm = getKeyAlgo(k.Key, false)
		}
		if k.Algorithm == "" && settings.RequireAlgorithm {
			// Algorithm could not be guessed, but it is a mandatory field
			return errors.New("could not detect algorithm for specified key")
		}
	}

	// Generate unique (hash-based) Key ID only if empty
	if k.KeyID == "" {
		var err error
		var thumbprint []byte
		if settings.ThumbprintHashFunc == nil {
			thumbprint, err = k.Thumbprint()
		} else {
			thumbprint, err = k.ThumbprintUsing(settings.ThumbprintHashFunc)
		}
		if err != nil {
			return err
		}
		if thumbprint != nil {
			// We succeeded generating a thumbprint for the key: encode it with Base64URL and use result as Key ID.
			k.KeyID = base64.RawURLEncoding.EncodeToString(thumbprint)
		} else if settings.RequireKeyID {
			// We failed generating a thumbprint for the key, but Key ID is mandatory
			return errors.New("key has no key ID specified and key ID generation from thumbprint failed")
		}
	}

	return nil
}
