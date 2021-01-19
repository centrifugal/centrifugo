package jwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
)

// NewSignerPS returns a new RSA-PSS-based signer.
func NewSignerPS(alg Algorithm, key *rsa.PrivateKey) (Signer, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	hash, opts, ok := getParamsPS(alg)
	if !ok {
		return nil, ErrUnsupportedAlg
	}
	return &psAlg{
		alg:        alg,
		hash:       hash,
		privateKey: key,
		opts:       opts,
	}, nil
}

// NewVerifierPS returns a new RSA-PSS-based signer.
func NewVerifierPS(alg Algorithm, key *rsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	hash, opts, ok := getParamsPS(alg)
	if !ok {
		return nil, ErrUnsupportedAlg
	}
	return &psAlg{
		alg:       alg,
		hash:      hash,
		publicKey: key,
		opts:      opts,
	}, nil
}

func getParamsPS(alg Algorithm) (crypto.Hash, *rsa.PSSOptions, bool) {
	switch alg {
	case PS256:
		return crypto.SHA256, optsPS256, true
	case PS384:
		return crypto.SHA384, optsPS384, true
	case PS512:
		return crypto.SHA512, optsPS512, true
	default:
		return 0, nil, false
	}
}

var (
	optsPS256 = &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
		Hash:       crypto.SHA256,
	}

	optsPS384 = &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
		Hash:       crypto.SHA384,
	}

	optsPS512 = &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
		Hash:       crypto.SHA512,
	}
)

type psAlg struct {
	alg        Algorithm
	hash       crypto.Hash
	publicKey  *rsa.PublicKey
	privateKey *rsa.PrivateKey
	opts       *rsa.PSSOptions
}

func (ps psAlg) SignSize() int {
	return ps.privateKey.Size()
}

func (ps psAlg) Algorithm() Algorithm {
	return ps.alg
}

func (ps psAlg) Sign(payload []byte) ([]byte, error) {
	digest, err := hashPayload(ps.hash, payload)
	if err != nil {
		return nil, err
	}

	signature, errSign := rsa.SignPSS(rand.Reader, ps.privateKey, ps.hash, digest, ps.opts)
	if errSign != nil {
		return nil, errSign
	}
	return signature, nil
}

func (ps psAlg) Verify(payload, signature []byte) error {
	digest, err := hashPayload(ps.hash, payload)
	if err != nil {
		return err
	}

	errVerify := rsa.VerifyPSS(ps.publicKey, ps.hash, digest, signature, ps.opts)
	if errVerify != nil {
		return ErrInvalidSignature
	}
	return nil
}
