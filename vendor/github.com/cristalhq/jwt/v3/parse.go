package jwt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
)

// ParseString decodes a token.
func ParseString(raw string) (*Token, error) {
	return Parse([]byte(raw))
}

// Parse decodes a token from a raw bytes.
func Parse(raw []byte) (*Token, error) {
	return parse(raw)
}

// ParseAndVerifyString decodes a token and verifies it's signature.
func ParseAndVerifyString(raw string, verifier Verifier) (*Token, error) {
	return ParseAndVerify([]byte(raw), verifier)
}

// ParseAndVerify decodes a token and verifies it's signature.
func ParseAndVerify(raw []byte, verifier Verifier) (*Token, error) {
	token, err := parse(raw)
	if err != nil {
		return nil, err
	}
	if !constTimeAlgEqual(token.Header().Algorithm, verifier.Algorithm()) {
		return nil, ErrAlgorithmMismatch
	}
	if err := verifier.Verify(token.Payload(), token.Signature()); err != nil {
		return nil, err
	}
	return token, nil
}

func parse(token []byte) (*Token, error) {
	dot1 := bytes.IndexByte(token, '.')
	dot2 := bytes.LastIndexByte(token, '.')
	if dot2 <= dot1 {
		return nil, ErrInvalidFormat
	}

	buf := make([]byte, len(token))

	headerN, err := b64Decode(buf, token[:dot1])
	if err != nil {
		return nil, ErrInvalidFormat
	}
	var header Header
	if err := json.Unmarshal(buf[:headerN], &header); err != nil {
		return nil, ErrInvalidFormat
	}

	claimsN, err := b64Decode(buf[headerN:], token[dot1+1:dot2])
	if err != nil {
		return nil, ErrInvalidFormat
	}
	claims := buf[headerN : headerN+claimsN]

	signN, err := b64Decode(buf[headerN+claimsN:], token[dot2+1:])
	if err != nil {
		return nil, ErrInvalidFormat
	}
	signature := buf[headerN+claimsN : headerN+claimsN+signN]

	tk := &Token{
		raw:       token,
		dot1:      dot1,
		dot2:      dot2,
		signature: signature,
		header:    header,
		claims:    claims,
	}
	return tk, nil
}

var b64Decode = base64.RawURLEncoding.Decode
