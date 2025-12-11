package tools

import "encoding/json"

// IsValidJSON checks if the given byte slice is valid JSON.
func IsValidJSON(data []byte) bool {
	return json.Valid(data)
}
