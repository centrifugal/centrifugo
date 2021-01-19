package jwk

import (
	"encoding/base64"
	"encoding/json"
)

type orderedJsonMarshaller struct {
	started bool
	buffer  []byte
}

func newOrderedJsonMarshaller(initialCapacity int) orderedJsonMarshaller {
	buffer := make([]byte, 0, initialCapacity)
	buffer = append(buffer, '{')
	return orderedJsonMarshaller{
		started: false,
		buffer:  buffer,
	}
}

func (m *orderedJsonMarshaller) marshalString(name string, value string) error {
	if value == "" {
		return nil // Do not write empty values
	}
	quotedValue, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.marshalKeyName(name)
	m.buffer = append(m.buffer, quotedValue...)
	return nil
}

func (m *orderedJsonMarshaller) marshalBytes(name string, value *keyBytes) {
	if value == nil {
		return
	}

	byteData := value.data

	if len(byteData) == 0 {
		return
	}

	m.marshalKeyName(name)
	m.buffer = append(m.buffer, '"')
	encodedValueLen := base64.RawURLEncoding.EncodedLen(len(byteData))
	valueIndex := len(m.buffer)
	m.buffer = append(m.buffer, make([]byte, encodedValueLen)...)
	base64.RawURLEncoding.Encode(m.buffer[valueIndex:], byteData)
	m.buffer = append(m.buffer, '"')
}

func (m *orderedJsonMarshaller) marshalKeyName(name string) {
	if m.started {
		m.buffer = append(m.buffer, ',') // Add comma
	} else {
		m.started = true
	}

	m.buffer = append(m.buffer, '"')
	m.buffer = append(m.buffer, name...)
	m.buffer = append(m.buffer, '"', ':')
}

func (m *orderedJsonMarshaller) finalize() []byte {
	m.buffer = append(m.buffer, '}')
	return m.buffer
}
