package jwtutils

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

var (
	errKeyMustBePEMEncoded = errors.New("key must be PEM encoded")
	errNotRSAPublicKey     = errors.New("key is not a valid RSA public key")
	errNotECDSAPublicKey   = errors.New("key is not a valid ECDSA public key")
	errNotEd25519PublicKey = errors.New("key is not a valid Ed25519 public key")
)

// ParseRSAPublicKeyFromPEM parses PEM encoded PKCS1 or PKCS8 public key.
func ParseRSAPublicKeyFromPEM(key []byte) (*rsa.PublicKey, error) {
	var err error

	var block *pem.Block
	if block, _ = pem.Decode(key); block == nil {
		return nil, errKeyMustBePEMEncoded
	}

	var parsedKey any
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

// ParseECDSAPublicKeyFromPEM parses PEM encoded public key.
func ParseECDSAPublicKeyFromPEM(key []byte) (*ecdsa.PublicKey, error) {
	var err error

	var block *pem.Block
	if block, _ = pem.Decode(key); block == nil {
		return nil, errKeyMustBePEMEncoded
	}

	var parsedKey any
	if parsedKey, err = x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			parsedKey = cert.PublicKey
		} else {
			return nil, err
		}
	}

	var pkey *ecdsa.PublicKey
	var ok bool
	if pkey, ok = parsedKey.(*ecdsa.PublicKey); !ok {
		return nil, errNotECDSAPublicKey
	}

	return pkey, nil
}

// ParseEd25519PublicKeyFromPEM parses PEM encoded PKIX Ed25519 public key.
func ParseEd25519PublicKeyFromPEM(key []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(key)
	if block == nil {
		return nil, errKeyMustBePEMEncoded
	}
	parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		cert, certErr := x509.ParseCertificate(block.Bytes)
		if certErr != nil {
			return nil, err
		}
		parsedKey = cert.PublicKey
	}
	pkey, ok := parsedKey.(ed25519.PublicKey)
	if !ok {
		return nil, errNotEd25519PublicKey
	}
	return pkey, nil
}
