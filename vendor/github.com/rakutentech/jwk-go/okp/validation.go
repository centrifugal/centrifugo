package okp

import (
	"bytes"

	"errors"

	"fmt"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
)

const (
	// Curve25519KeySize is the size of both Curve25519 public and private keys
	Curve25519KeySize = 32

	// Ed25519KeySize is the size of both Ed25519 public and private keys
	Ed25519KeySize = 32
)

func checkKeySize(keyType string, key []byte, expectedSize int) error {
	keySize := len(key)
	if keySize == 0 {
		return nil // Key is empty, no need to check size
	} else if keySize != expectedSize {
		return fmt.Errorf("expected %s key to %d bytes long, got %d bytes",
			keyType, expectedSize, keySize)
	}
	return nil
}

func validateEd25519(kp OctetKeyPairBase) (err error) {
	if kp.publicKey == nil && kp.privateKey == nil {
		return ErrKeyMissing // OKP must have public or private key to be valid
	}
	err = checkKeySize("private", kp.privateKey, Ed25519KeySize)
	if err != nil {
		return
	}
	err = checkKeySize("public", kp.publicKey, Ed25519KeySize)
	if err != nil {
		return
	}
	// Derive public key from private key
	if kp.publicKey == nil {
		kp.publicKey, err = derivePublicEd25519FromPrivate(kp.privateKey)
	}
	return
}

func validateCurve25519(kp OctetKeyPairBase) (err error) {
	if kp.publicKey == nil && kp.privateKey == nil {
		return ErrKeyMissing // OKP must have public or private key to be valid
	}
	err = checkKeySize("private", kp.privateKey, Curve25519KeySize)
	if err != nil {
		return
	}
	err = checkKeySize("public", kp.publicKey, Curve25519KeySize)
	if err != nil {
		return
	}
	// Derive public key from private key
	if kp.publicKey == nil {
		publicKey := new([32]byte)
		privateKey := new([32]byte)
		copy(privateKey[:], kp.privateKey)
		curve25519.ScalarBaseMult(publicKey, privateKey)
		kp.publicKey = publicKey[:]
	}
	return
}

func derivePublicEd25519FromPrivate(privateKey []byte) ([]byte, error) {
	if len(privateKey) < 32 {
		return nil, errors.New("Ed25519 Private key must be at least 32 bytes long")
	}
	pub, _, err := ed25519.GenerateKey(bytes.NewReader(privateKey))
	return pub, err
}
