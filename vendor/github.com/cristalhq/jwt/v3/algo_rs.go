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
	hash, ok := getHashRSA(alg)
	if !ok {
		return nil, ErrUnsupportedAlg
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
	hash, ok := getHashRSA(alg)
	if !ok {
		return nil, ErrUnsupportedAlg
	}
	return &rsAlg{
		alg:       alg,
		hash:      hash,
		publickey: key,
	}, nil
}

func getHashRSA(alg Algorithm) (crypto.Hash, bool) {
	switch alg {
	case RS256:
		return crypto.SHA256, true
	case RS384:
		return crypto.SHA384, true
	case RS512:
		return crypto.SHA512, true
	default:
		return 0, false
	}
}

type rsAlg struct {
	alg        Algorithm
	hash       crypto.Hash
	publickey  *rsa.PublicKey
	privateKey *rsa.PrivateKey
}

func (rs rsAlg) Algorithm() Algorithm {
	return rs.alg
}

func (rs rsAlg) SignSize() int {
	return rs.privateKey.Size()
}

func (rs rsAlg) Sign(payload []byte) ([]byte, error) {
	digest, err := hashPayload(rs.hash, payload)
	if err != nil {
		return nil, err
	}

	signature, errSign := rsa.SignPKCS1v15(rand.Reader, rs.privateKey, rs.hash, digest)
	if errSign != nil {
		return nil, errSign
	}
	return signature, nil
}

func (rs rsAlg) Verify(payload, signature []byte) error {
	digest, err := hashPayload(rs.hash, payload)
	if err != nil {
		return err
	}

	errVerify := rsa.VerifyPKCS1v15(rs.publickey, rs.hash, digest, signature)
	if errVerify != nil {
		return ErrInvalidSignature
	}
	return nil
}
