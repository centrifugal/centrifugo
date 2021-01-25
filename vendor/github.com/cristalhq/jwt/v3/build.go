package jwt

import (
	"encoding/base64"
	"encoding/json"
)

// BuilderOption is used to modify builder properties.
type BuilderOption func(*Builder)

// WithKeyID sets `kid` header for token.
func WithKeyID(kid string) BuilderOption {
	return func(b *Builder) { b.header.KeyID = kid }
}

// WithContentType sets `cty` header for token.
func WithContentType(cty string) BuilderOption {
	return func(b *Builder) { b.header.ContentType = cty }
}

// Builder is used to create a new token.
type Builder struct {
	signer    Signer
	header    Header
	headerRaw []byte
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
func NewBuilder(signer Signer, opts ...BuilderOption) *Builder {
	b := &Builder{
		signer: signer,
		header: Header{
			Algorithm: signer.Algorithm(),
			Type:      "JWT",
		},
	}

	for _, opt := range opts {
		opt(b)
	}

	b.headerRaw = encodeHeader(b.header)
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
// If claims param is of type []byte or string then it's treated as a marshaled JSON.
// In other words you can pass already marshaled claims.
//
func (b *Builder) Build(claims interface{}) (*Token, error) {
	rawClaims, errClaims := encodeClaims(claims)
	if errClaims != nil {
		return nil, errClaims
	}

	lenH := len(b.headerRaw)
	lenC := b64EncodedLen(len(rawClaims))
	lenS := b64EncodedLen(b.signer.SignSize())

	token := make([]byte, lenH+1+lenC+1+lenS)
	idx := 0
	idx = copy(token[idx:], b.headerRaw)

	// add '.' and append encoded claims
	token[idx] = '.'
	idx++
	b64Encode(token[idx:], rawClaims)
	idx += lenC

	// calculate signature of already written 'header.claims'
	rawSignature, errSign := b.signer.Sign(token[:idx])
	if errSign != nil {
		return nil, errSign
	}

	// add '.' and append encoded signature
	token[idx] = '.'
	idx++
	b64Encode(token[idx:], rawSignature)

	t := &Token{
		raw:       token,
		dot1:      lenH,
		dot2:      lenH + 1 + lenC,
		header:    b.header,
		claims:    rawClaims,
		signature: rawSignature,
	}
	return t, nil
}

func encodeClaims(claims interface{}) ([]byte, error) {
	switch claims := claims.(type) {
	case []byte:
		return claims, nil
	case string:
		return []byte(claims), nil
	default:
		return json.Marshal(claims)
	}
}

func encodeHeader(header Header) []byte {
	if header.Type == "JWT" && header.ContentType == "" && header.KeyID == "" {
		if h := getPredefinedHeader(header); h != "" {
			return []byte(h)
		}
		// another algorithm? encode below
	}
	// returned err is always nil, see *Header.MarshalJSON
	buf, _ := json.Marshal(header)

	encoded := make([]byte, b64EncodedLen(len(buf)))
	b64Encode(encoded, buf)
	return encoded
}

func getPredefinedHeader(header Header) string {
	switch header.Algorithm {
	case EdDSA:
		return "eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9"

	case HS256:
		return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	case HS384:
		return "eyJhbGciOiJIUzM4NCIsInR5cCI6IkpXVCJ9"
	case HS512:
		return "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9"

	case RS256:
		return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	case RS384:
		return "eyJhbGciOiJSUzM4NCIsInR5cCI6IkpXVCJ9"
	case RS512:
		return "eyJhbGciOiJSUzUxMiIsInR5cCI6IkpXVCJ9"

	case ES256:
		return "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9"
	case ES384:
		return "eyJhbGciOiJFUzM4NCIsInR5cCI6IkpXVCJ9"
	case ES512:
		return "eyJhbGciOiJFUzUxMiIsInR5cCI6IkpXVCJ9"

	case PS256:
		return "eyJhbGciOiJQUzI1NiIsInR5cCI6IkpXVCJ9"
	case PS384:
		return "eyJhbGciOiJQUzM4NCIsInR5cCI6IkpXVCJ9"
	case PS512:
		return "eyJhbGciOiJQUzUxMiIsInR5cCI6IkpXVCJ9"

	default:
		return ""
	}
}

var (
	b64Encode     = base64.RawURLEncoding.Encode
	b64EncodedLen = base64.RawURLEncoding.EncodedLen
)
