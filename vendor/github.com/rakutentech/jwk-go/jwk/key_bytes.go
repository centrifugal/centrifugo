package jwk

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
)

// keyBytes represents a slice of bytes that can be serialized to url-safe base64.
type keyBytes struct {
	data []byte
}

func keyBytesFrom(data []byte) *keyBytes {
	if data == nil {
		return nil
	}
	return &keyBytes{
		data: data,
	}
}

func keyBytesFromUInt64(i uint64) *keyBytes {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, i)
	for i := range data {
		if data[i] != 0 {
			return &keyBytes{data[i:]}
		}
	}
	return nil
}

func (kb *keyBytes) padTo(numBytes int) *keyBytes {
	var b []byte
	if kb == nil {
		b = nil
	} else {
		b = kb.data
	}

	padded := make([]byte, numBytes)
	copy(padded[numBytes-len(b):], b)
	return &keyBytes{padded}
}

func (kb *keyBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(kb.toBase64())
}

func (kb *keyBytes) UnmarshalJSON(data []byte) error {
	var b64urlStr string
	err := json.Unmarshal(data, &b64urlStr)
	if err != nil {
		return err
	}

	// Reset keyBytes to empty if string is empty
	if b64urlStr == "" && len(kb.data) != 0 {
		kb.data = nil
		return nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(b64urlStr)
	if err != nil {
		return err
	}

	kb.data = decoded

	return nil
}

func (kb *keyBytes) toBigInt() *big.Int {
	if kb.data == nil {
		return nil
	}
	var bi big.Int
	bi.SetBytes(kb.data)
	return &bi
}

func (kb *keyBytes) toBase64() string {
	return base64.RawURLEncoding.EncodeToString(kb.data)
}
