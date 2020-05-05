package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

var (
	errKeyMustBePEMEncoded = errors.New("key must be PEM encoded")
	errNotRSAPublicKey     = errors.New("key is not a valid RSA public key")
)

// ParseRSAPublicKeyFromPEM parses PEM encoded PKCS1 or PKCS8 public key.
func ParseRSAPublicKeyFromPEM(key []byte) (*rsa.PublicKey, error) {
	var err error

	var block *pem.Block
	if block, _ = pem.Decode(key); block == nil {
		return nil, errKeyMustBePEMEncoded
	}

	var parsedKey interface{}
	if parsedKey, err = x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			parsedKey = cert.PublicKey
		} else {
			return nil, err
		}
	}

	var pkey *rsa.PublicKey
	var ok bool
	if pkey, ok = parsedKey.(*rsa.PublicKey); !ok {
		return nil, errNotRSAPublicKey
	}

	return pkey, nil
}
