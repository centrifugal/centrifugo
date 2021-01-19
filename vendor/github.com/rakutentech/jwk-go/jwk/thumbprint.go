package jwk

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"hash"

	"github.com/rakutentech/jwk-go/okp"
)

// Thumbprint returns the KeySpec's RFC 7638 thumbprint using SHA-256.
func (k *KeySpec) Thumbprint() ([]byte, error) {
	return getKeyThumbprint(k.Key, sha256.New())
}

// ThumbprintUsing returns the KeySpec's RFC 7638 thumbprint using the
// specified hash function.
func (k *KeySpec) ThumbprintUsing(h hash.Hash) ([]byte, error) {
	return getKeyThumbprint(k.Key, h)
}

const rsaThumb = `{"e":"%s","kty":"RSA","n":"%s"}`
const ecThumb = `{"crv":"%s","kty":"EC","x":"%s","y":"%s"}`
const octThumb = `{"k":"%s","kty":"oct"}`
const okpThumb = `{"crv":"%s","kty":"OKP","x":"%s"}`

func writeRSAThumbprint(w io.Writer, key *rsa.PublicKey) error {
	k, err := fromRSAPublic(key)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, rsaThumb, k.E.toBase64(), k.N.toBase64())
	return nil
}

func writeECThumbprint(w io.Writer, key *ecdsa.PublicKey) error {
	k, err := fromECPublic(key)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, ecThumb, k.Crv, k.X.toBase64(), k.Y.toBase64())
	return nil
}

func writeOctThumbprint(w io.Writer, key []byte) {
	fmt.Fprintf(w, octThumb, base64.RawURLEncoding.EncodeToString(key))
}

func writeCurveOKPThumbprint(w io.Writer, key okp.CurveOctetKeyPair) {
	fmt.Fprintf(w, okpThumb, key.Curve(), base64.RawURLEncoding.EncodeToString(key.PublicKey()))
}

func writeAnyThumbprint(w io.Writer, key interface{}) error {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return writeRSAThumbprint(w, &k.PublicKey)
	case *rsa.PublicKey:
		return writeRSAThumbprint(w, k)
	case *ecdsa.PrivateKey:
		return writeECThumbprint(w, &k.PublicKey)
	case *ecdsa.PublicKey:
		return writeECThumbprint(w, k)
	case okp.CurveOctetKeyPair:
		writeCurveOKPThumbprint(w, k)
	case []byte:
		writeOctThumbprint(w, k)
	default:
		return errors.New("unsupported key type for thumbprints")
	}
	return nil
}

func getKeyThumbprint(key interface{}, h hash.Hash) ([]byte, error) {
	err := writeAnyThumbprint(h, key)
	if err != nil {
		return nil, err
	}
	thumbprint := h.Sum(make([]byte, 0, sha256.Size))
	return thumbprint, nil
}
