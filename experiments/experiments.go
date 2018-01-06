package experiments

import "encoding/json"

// Command ...
type Command struct {
	ID     int             `json:"i"`
	Method string          `json:"m"`
	Params json.RawMessage `json:"p"`
}
