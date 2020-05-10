package jwt

import (
	"crypto/ed25519"
)

// NewSignerEdDSA returns a new ed25519-based signer.
func NewSignerEdDSA(key ed25519.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	return &edDSAAlg{
		alg:        EdDSA,
		privateKey: key,
	}, nil
}

// NewVerifierEdDSA returns a new ed25519-based verifier.
func NewVerifierEdDSA(key ed25519.PublicKey) (Verifier, error) {
	if key == nil {
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

func (h edDSAAlg) Algorithm() Algorithm {
	return h.alg
}

func (h edDSAAlg) Sign(payload []byte) ([]byte, error) {
	return ed25519.Sign(h.privateKey, payload), nil
}

func (h edDSAAlg) Verify(payload, signature []byte) error {
	if !ed25519.Verify(h.publicKey, payload, signature) {
		return ErrInvalidSignature
	}
	return nil
}
