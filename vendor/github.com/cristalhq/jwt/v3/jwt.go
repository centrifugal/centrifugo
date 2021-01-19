package jwt

import (
	"bytes"
	"encoding/json"
)

// Token represents a JWT token.
// See: https://tools.ietf.org/html/rfc7519
//
type Token struct {
	raw       []byte
	dot1      int
	dot2      int
	signature []byte
	header    Header
	claims    json.RawMessage
}

func (t *Token) String() string {
	return string(t.raw)
}

// SecureString returns token without a signature (replaced with `.<signature>`).
func (t *Token) SecureString() string {
	dot := bytes.LastIndexByte(t.raw, '.')
	return string(t.raw[:dot]) + `.<signature>`
}

// Raw returns token's raw bytes.
func (t *Token) Raw() []byte {
	return t.raw
}

// Header returns token's header.
func (t *Token) Header() Header {
	return t.header
}

// RawHeader returns token's header raw bytes.
func (t *Token) RawHeader() []byte {
	return t.raw[:t.dot1]
}

// RawClaims returns token's claims as a raw bytes.
func (t *Token) RawClaims() []byte {
	return t.claims
}

// Payload returns token's payload.
func (t *Token) Payload() []byte {
	return t.raw[:t.dot2]
}

// Signature returns token's signature.
func (t *Token) Signature() []byte {
	return t.signature
}

// Header representa JWT header data.
// See: https://tools.ietf.org/html/rfc7519#section-5, https://tools.ietf.org/html/rfc7517
//
type Header struct {
	Algorithm   Algorithm `json:"alg"`
	Type        string    `json:"typ,omitempty"` // only "JWT" can be here
	ContentType string    `json:"cty,omitempty"`
	KeyID       string    `json:"kid,omitempty"`
}

// MarshalJSON implements the json.Marshaler interface.
func (h *Header) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString(`{"alg":"`)
	buf.WriteString(string(h.Algorithm))

	if h.Type != "" {
		buf.WriteString(`","typ":"`)
		buf.WriteString(h.Type)
	}
	if h.ContentType != "" {
		buf.WriteString(`","cty":"`)
		buf.WriteString(h.ContentType)
	}
	if h.KeyID != "" {
		buf.WriteString(`","kid":"`)
		buf.WriteString(h.KeyID)
	}
	buf.WriteString(`"}`)

	return buf.Bytes(), nil
}
