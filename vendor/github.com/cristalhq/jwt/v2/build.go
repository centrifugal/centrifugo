package jwt

import (
	"encoding/base64"
	"encoding/json"
)

var (
	base64Encode     = base64.RawURLEncoding.Encode
	base64EncodedLen = base64.RawURLEncoding.EncodedLen
)

// Builder is used to create a new token.
type Builder struct {
	signer Signer
	header Header
}

// BuildBytes is used to create and encode JWT with a provided claims.
func BuildBytes(signer Signer, claims interface{}) ([]byte, error) {
	return NewBuilder(signer).BuildBytes(claims)
}

// Build is used to create and encode JWT with a provided claims.
func Build(signer Signer, claims interface{}) (*Token, error) {
	return NewBuilder(signer).Build(claims)
}

// NewBuilder returns new instance of Builder.
func NewBuilder(signer Signer) *Builder {
	b := &Builder{
		signer: signer,

		header: Header{
			Algorithm: signer.Algorithm(),
			Type:      "JWT",
		},
	}
	return b
}

// BuildBytes used to create and encode JWT with a provided claims.
func (b *Builder) BuildBytes(claims interface{}) ([]byte, error) {
	token, err := b.Build(claims)
	if err != nil {
		return nil, err
	}
	return token.Raw(), nil
}

// Build used to create and encode JWT with a provided claims.
func (b *Builder) Build(claims interface{}) (*Token, error) {
	rawClaims, encodedClaims, err := encodeClaims(claims)
	if err != nil {
		return nil, err
	}

	encodedHeader := encodeHeader(&b.header)
	payload := concatParts(encodedHeader, encodedClaims)

	raw, signature, err := signPayload(b.signer, payload)
	if err != nil {
		return nil, err
	}

	token := &Token{
		raw:       raw,
		payload:   payload,
		signature: signature,
		header:    b.header,
		claims:    rawClaims,
	}
	return token, nil
}

func encodeClaims(claims interface{}) (raw, encoded []byte, err error) {
	raw, err = json.Marshal(claims)
	if err != nil {
		return nil, nil, err
	}

	encoded = make([]byte, base64EncodedLen(len(raw)))
	base64Encode(encoded, raw)

	return raw, encoded, nil
}

func encodeHeader(header *Header) []byte {
	// returned err is always nil, see *Header.MarshalJSON
	buf, _ := header.MarshalJSON()

	encoded := make([]byte, base64EncodedLen(len(buf)))
	base64Encode(encoded, buf)

	return encoded
}

func signPayload(signer Signer, payload []byte) (signed, signature []byte, err error) {
	signature, err = signer.Sign(payload)
	if err != nil {
		return nil, nil, err
	}

	encodedSignature := make([]byte, base64EncodedLen(len(signature)))
	base64Encode(encodedSignature, signature)

	signed = concatParts(payload, encodedSignature)

	return signed, signature, nil
}

func concatParts(a, b []byte) []byte {
	buf := make([]byte, len(a)+1+len(b))
	buf[len(a)] = '.'

	copy(buf[:len(a)], a)
	copy(buf[len(a)+1:], b)

	return buf
}
