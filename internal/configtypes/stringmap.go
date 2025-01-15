package configtypes

import "encoding/json"

type MapStringString map[string]string

func (s *MapStringString) Decode(value string) error {
	var m map[string]string
	err := json.Unmarshal([]byte(value), &m)
	*s = m
	return err
}
