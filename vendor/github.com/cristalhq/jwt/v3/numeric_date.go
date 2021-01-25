package jwt

import (
	"encoding/json"
	"math"
	"strconv"
	"time"
)

// NumericDate represents date for StandardClaims
// See: https://tools.ietf.org/html/rfc7519#section-2
//
type NumericDate struct {
	time.Time
}

// NewNumericDate creates a new NumericDate value from time.Time.
func NewNumericDate(t time.Time) *NumericDate {
	if t.IsZero() {
		return nil
	}
	return &NumericDate{t}
}

// MarshalJSON implements the json.Marshaler interface.
func (t *NumericDate) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(t.Unix(), 10)), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *NumericDate) UnmarshalJSON(data []byte) error {
	var value json.Number
	if err := json.Unmarshal(data, &value); err != nil {
		return ErrDateInvalidFormat
	}
	f, err := value.Float64()
	if err != nil {
		return ErrDateInvalidFormat
	}
	sec, dec := math.Modf(f)
	ts := time.Unix(int64(sec), int64(dec*1e9))
	*t = NumericDate{ts}
	return nil
}
