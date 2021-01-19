package jwk

// JWK represents an unparsed JSON Web Key (JWK) in its wire format.
type JWK struct {
	Kid string `json:"kid,omitempty"`
	Kty string `json:"kty,omitempty"`
	Alg string `json:"alg,omitempty"`
	Crv string `json:"crv,omitempty"`
	Use string `json:"use,omitempty"`

	// Public Fields
	X *keyBytes `json:"x,omitempty"`
	Y *keyBytes `json:"y,omitempty"`
	N *keyBytes `json:"n,omitempty"`
	E *keyBytes `json:"e,omitempty"`

	// Private Fields
	D *keyBytes `json:"d,omitempty"`
	P *keyBytes `json:"p,omitempty"`
	Q *keyBytes `json:"q,omitempty"`
	Dp *keyBytes `json:"dp,omitempty"`
	Dq *keyBytes `json:"dq,omitempty"`
	Qi *keyBytes `json:"qi,omitempty"`

	// Symmetric Keys
	K *keyBytes `json:"k,omitempty"`
}

func (jwk *JWK) MarshalJSON() ([]byte, error) {
	m := newOrderedJsonMarshaller(128)

	var err error
	err = m.marshalString("kid", jwk.Kid)
	if err != nil {
		return nil, err
	}
	err = m.marshalString("kty", jwk.Kty)
	if err != nil {
		return nil, err
	}
	err = m.marshalString("use", jwk.Use)
	if err != nil {
		return nil, err
	}
	err = m.marshalString("alg", jwk.Alg)
	if err != nil {
		return nil, err
	}
	err = m.marshalString("crv", jwk.Crv)
	if err != nil {
		return nil, err
	}
	m.marshalBytes("x", jwk.X)
	m.marshalBytes("y", jwk.Y)
	m.marshalBytes("n", jwk.N)
	m.marshalBytes("e", jwk.E)
	m.marshalBytes("d", jwk.D)
	m.marshalBytes("p", jwk.P)
	m.marshalBytes("q", jwk.Q)
	m.marshalBytes("k", jwk.K)
	m.marshalBytes("dp", jwk.Dp)
	m.marshalBytes("dq", jwk.Dq)
	m.marshalBytes("qi", jwk.Qi)

	data := m.finalize()

	return data, nil
}
