package jwt

import "encoding/json"

// Audience is a special claim that be a single string or an array of strings
// see RFC 7519.
type Audience []string

// MarshalJSON implements a marshaling function for "aud" claim.
func (a Audience) MarshalJSON() ([]byte, error) {
	switch len(a) {
	case 0:
		return []byte(`""`), nil
	case 1:
		return json.Marshal(a[0])
	default:
		return json.Marshal([]string(a))
	}
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (a *Audience) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return ErrAudienceInvalidFormat
	}

	switch v := v.(type) {
	case string:
		*a = Audience{v}
		return nil
	case []interface{}:
		aud := make(Audience, len(v))
		for i := range v {
			v, ok := v[i].(string)
			if !ok {
				return ErrAudienceInvalidFormat
			}
			aud[i] = v
		}
		*a = aud
		return nil
	default:
		return ErrAudienceInvalidFormat
	}
}
