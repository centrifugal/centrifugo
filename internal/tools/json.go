package tools

import "encoding/json"

// IsValidJSON checks if the given byte slice is valid JSON.
func IsValidJSON(data []byte) bool {
	return json.Valid(data)
}

// IsValidJSONObject checks if the given byte slice is valid JSON and specifically a JSON object.
func IsValidJSONObject(data []byte) bool {
	if !IsValidJSON(data) {
		return false
	}
	// Find first non-whitespace character
	start := 0
	for start < len(data) && isWhitespace(data[start]) {
		start++
	}
	// Find last non-whitespace character
	end := len(data) - 1
	for end >= start && isWhitespace(data[end]) {
		end--
	}
	return start <= end && data[start] == '{' && data[end] == '}'
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
