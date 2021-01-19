package jwt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
)

var b64Decode = base64.RawURLEncoding.Decode

// ParseString decodes a token.
func ParseString(raw string) (*Token, error) {
	return Parse([]byte(raw))
}

// Parse decodes a token from a raw bytes.
func Parse(raw []byte) (*Token, error) {
	dot1 := bytes.IndexByte(raw, '.')
	dot2 := bytes.LastIndexByte(raw, '.')
	if dot2 <= dot1 {
		return nil, ErrInvalidFormat
	}

	buf := make([]byte, len(raw))

	headerN, err := b64Decode(buf, raw[:dot1])
	if err != nil {
		return nil, ErrInvalidFormat
	}
	var header Header
	if err := json.Unmarshal(buf[:headerN], &header); err != nil {
		return nil, ErrInvalidFormat
	}

	claimsN, err := b64Decode(buf[headerN:], raw[dot1+1:dot2])
	if err != nil {
		return nil, ErrInvalidFormat
	}
	claims := buf[headerN : headerN+claimsN]

	signN, err := b64Decode(buf[headerN+claimsN:], raw[dot2+1:])
	if err != nil {
		return nil, ErrInvalidFormat
	}
	signature := buf[headerN+claimsN : headerN+claimsN+signN]

	token := &Token{
		raw:       raw,
		dot1:      dot1,
		dot2:      dot2,
		signature: signature,
		header:    header,
		claims:    claims,
	}
	return token, nil
}

// ParseAndVerifyString decodes a token and verifies it's signature.
func ParseAndVerifyString(raw string, verifier Verifier) (*Token, error) {
	return ParseAndVerify([]byte(raw), verifier)
}

// ParseAndVerify decodes a token and verifies it's signature.
func ParseAndVerify(raw []byte, verifier Verifier) (*Token, error) {
	token, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	if token.Header().Algorithm != verifier.Algorithm() {
		return nil, ErrAlgorithmMismatch
	}
	if err := verifier.Verify(token.Payload(), token.Signature()); err != nil {
		return nil, err
	}
	return token, nil
}
