package jwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
)

// NewSignerRS returns a new RSA-based signer.
func NewSignerRS(alg Algorithm, key *rsa.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	hash, err := getHashRSA(alg)
	if err != nil {
		return nil, err
	}
	return &rsAlg{
		alg:        alg,
		hash:       hash,
		privateKey: key,
	}, nil
}

// NewVerifierRS returns a new RSA-based verifier.
func NewVerifierRS(alg Algorithm, key *rsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	hash, err := getHashRSA(alg)
	if err != nil {
		return nil, err
	}
	return &rsAlg{
		alg:       alg,
		hash:      hash,
		publickey: key,
	}, nil
}

func getHashRSA(alg Algorithm) (crypto.Hash, error) {
	switch alg {
	case RS256:
		return crypto.SHA256, nil
	case RS384:
		return crypto.SHA384, nil
	case RS512:
		return crypto.SHA512, nil
	default:
		return 0, ErrUnsupportedAlg
	}
}

type rsAlg struct {
	alg        Algorithm
	hash       crypto.Hash
	publickey  *rsa.PublicKey
	privateKey *rsa.PrivateKey
}

func (h rsAlg) Algorithm() Algorithm {
	return h.alg
}

func (h rsAlg) SignSize() int {
	return h.privateKey.Size()
}

func (h rsAlg) Sign(payload []byte) ([]byte, error) {
	signed, err := h.sign(payload)
	if err != nil {
		return nil, err
	}

	signature, err := rsa.SignPKCS1v15(rand.Reader, h.privateKey, h.hash, signed)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func (h rsAlg) Verify(payload, signature []byte) error {
	signed, err := h.sign(payload)
	if err != nil {
		return err
	}

	err = rsa.VerifyPKCS1v15(h.publickey, h.hash, signed, signature)
	if err != nil {
		return ErrInvalidSignature
	}
	return nil
}

func (h rsAlg) sign(payload []byte) ([]byte, error) {
	hasher := h.hash.New()

	_, err := hasher.Write(payload)
	if err != nil {
		return nil, err
	}
	signed := hasher.Sum(nil)
	return signed, nil
}
