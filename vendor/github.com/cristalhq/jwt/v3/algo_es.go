package jwt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"math/big"
)

// NewSignerES returns a new ECDSA-based signer.
func NewSignerES(alg Algorithm, key *ecdsa.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	hash, ok := getParamsES(alg)
	if !ok {
		return nil, ErrUnsupportedAlg
	}
	return &esAlg{
		alg:        alg,
		hash:       hash,
		privateKey: key,
		signSize:   roundBytes(key.PublicKey.Params().BitSize) * 2,
	}, nil
}

// NewVerifierES returns a new ECDSA-based verifier.
func NewVerifierES(alg Algorithm, key *ecdsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	hash, ok := getParamsES(alg)
	if !ok {
		return nil, ErrUnsupportedAlg
	}
	return &esAlg{
		alg:       alg,
		hash:      hash,
		publickey: key,
		signSize:  roundBytes(key.Params().BitSize) * 2,
	}, nil
}

func getParamsES(alg Algorithm) (crypto.Hash, bool) {
	switch alg {
	case ES256:
		return crypto.SHA256, true
	case ES384:
		return crypto.SHA384, true
	case ES512:
		return crypto.SHA512, true
	default:
		return 0, false
	}
}

type esAlg struct {
	alg        Algorithm
	hash       crypto.Hash
	publickey  *ecdsa.PublicKey
	privateKey *ecdsa.PrivateKey
	signSize   int
}

func (es esAlg) Algorithm() Algorithm {
	return es.alg
}

func (es esAlg) SignSize() int {
	return es.signSize
}

func (es esAlg) Sign(payload []byte) ([]byte, error) {
	digest, err := hashPayload(es.hash, payload)
	if err != nil {
		return nil, err
	}

	r, s, errSign := ecdsa.Sign(rand.Reader, es.privateKey, digest)
	if err != nil {
		return nil, errSign
	}

	pivot := es.SignSize() / 2

	rBytes, sBytes := r.Bytes(), s.Bytes()
	signature := make([]byte, es.SignSize())
	copy(signature[pivot-len(rBytes):], rBytes)
	copy(signature[pivot*2-len(sBytes):], sBytes)
	return signature, nil
}

func (es esAlg) Verify(payload, signature []byte) error {
	if len(signature) != es.SignSize() {
		return ErrInvalidSignature
	}

	digest, err := hashPayload(es.hash, payload)
	if err != nil {
		return err
	}

	pivot := es.SignSize() / 2
	r := big.NewInt(0).SetBytes(signature[:pivot])
	s := big.NewInt(0).SetBytes(signature[pivot:])

	if !ecdsa.Verify(es.publickey, digest, r, s) {
		return ErrInvalidSignature
	}
	return nil
}

func roundBytes(n int) int {
	res := n / 8
	if n%8 > 0 {
		return res + 1
	}
	return res
}
