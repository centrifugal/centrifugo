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
	hash, keySize, curveBits, err := getParamsES(alg)
	if err != nil {
		return nil, err
	}
	return &esAlg{
		alg:        alg,
		hash:       hash,
		privateKey: key,
		keySize:    keySize,
		curveBits:  curveBits,
	}, nil
}

// NewVerifierES returns a new ECDSA-based verifier.
func NewVerifierES(alg Algorithm, key *ecdsa.PublicKey) (Verifier, error) {
	if key == nil {
		return nil, ErrInvalidKey
	}
	hash, keySize, curveBits, err := getParamsES(alg)
	if err != nil {
		return nil, err
	}
	return &esAlg{
		alg:       alg,
		hash:      hash,
		publickey: key,
		keySize:   keySize,
		curveBits: curveBits,
	}, nil
}

func getParamsES(alg Algorithm) (crypto.Hash, int, int, error) {
	switch alg {
	case ES256:
		return crypto.SHA256, 32, 256, nil
	case ES384:
		return crypto.SHA384, 48, 384, nil
	case ES512:
		return crypto.SHA512, 66, 521, nil
	default:
		return 0, 0, 0, ErrUnsupportedAlg
	}
}

type esAlg struct {
	alg        Algorithm
	hash       crypto.Hash
	publickey  *ecdsa.PublicKey
	privateKey *ecdsa.PrivateKey
	keySize    int
	curveBits  int
}

func (h esAlg) Algorithm() Algorithm {
	return h.alg
}

func (h esAlg) Sign(payload []byte) ([]byte, error) {
	signed, err := h.sign(payload)
	if err != nil {
		return nil, err
	}

	r, s, err := ecdsa.Sign(rand.Reader, h.privateKey, signed)
	if err != nil {
		return nil, err
	}
	curveBits := h.privateKey.Curve.Params().BitSize

	keyBytes := curveBits / 8
	if curveBits%8 > 0 {
		keyBytes++
	}

	// Serialize r and s into big-endian byte slices and round up size to keyBytes.
	rb := r.Bytes()
	rbPadded := make([]byte, keyBytes)
	copy(rbPadded[keyBytes-len(rb):], rb)

	sb := s.Bytes()
	sbPadded := make([]byte, keyBytes)
	copy(sbPadded[keyBytes-len(sb):], sb)

	out := append(rbPadded, sbPadded...)

	return out, nil
}

func (h esAlg) Verify(payload, signature []byte) error {
	if len(signature) != 2*h.keySize {
		return ErrInvalidSignature
	}

	signed, err := h.sign(payload)
	if err != nil {
		return err
	}

	r := big.NewInt(0).SetBytes(signature[:h.keySize])
	s := big.NewInt(0).SetBytes(signature[h.keySize:])

	if !ecdsa.Verify(h.publickey, signed, r, s) {
		return ErrInvalidSignature
	}
	return nil
}

func (h esAlg) sign(payload []byte) ([]byte, error) {
	hasher := h.hash.New()

	_, err := hasher.Write(payload)
	if err != nil {
		return nil, err
	}
	signed := hasher.Sum(nil)
	return signed, nil
}
