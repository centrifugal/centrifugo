package jwt

import (
	"crypto/ed25519"
)

// NewSignerEdDSA returns a new ed25519-based signer.
func NewSignerEdDSA(key ed25519.PrivateKey) (Signer, error) {
	if len(key) == 0 || len(key) != ed25519.PrivateKeySize {
		return nil, ErrInvalidKey
	}
	return &edDSAAlg{
		alg:        EdDSA,
		privateKey: key,
	}, nil
}

// NewVerifierEdDSA returns a new ed25519-based verifier.
func NewVerifierEdDSA(key ed25519.PublicKey) (Verifier, error) {
	if len(key) == 0 || len(key) != ed25519.PublicKeySize {
		return nil, ErrInvalidKey
	}
	return &edDSAAlg{
		alg:       EdDSA,
		publicKey: key,
	}, nil
}

type edDSAAlg struct {
	alg        Algorithm
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func (ed edDSAAlg) Algorithm() Algorithm {
	return ed.alg
}

func (ed edDSAAlg) SignSize() int {
	return ed25519.SignatureSize
}

func (ed edDSAAlg) Sign(payload []byte) ([]byte, error) {
	return ed25519.Sign(ed.privateKey, payload), nil
}

func (ed edDSAAlg) Verify(payload, signature []byte) error {
	if !ed25519.Verify(ed.publicKey, payload, signature) {
		return ErrInvalidSignature
	}
	return nil
}
