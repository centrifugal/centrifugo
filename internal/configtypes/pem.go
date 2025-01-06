package configtypes

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
)

// PEMData represents a flexible PEM-encoded source.
// The order sources checked is the following:
// 1. Raw PEM content
// 2. Base64 encoded PEM content
// 3. Path to file with PEM content
type PEMData string

// String converts PEMData to a string.
func (p PEMData) String() string {
	return string(p)
}

// MarshalJSON converts PEMData to JSON.
func (p PEMData) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(p))
}

// UnmarshalJSON parses PEMData from JSON.
func (p *PEMData) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*p = PEMData(str)
	return nil
}

// MarshalText converts PEMData to text for TOML.
func (p PEMData) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText parses PEMData from text (used in TOML).
func (p *PEMData) UnmarshalText(text []byte) error {
	*p = PEMData(text)
	return nil
}

// MarshalYAML converts PEMData to a YAML-compatible format.
func (p PEMData) MarshalYAML() (interface{}, error) {
	return p.String(), nil
}

// UnmarshalYAML parses PEMData from YAML.
func (p *PEMData) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}
	*p = PEMData(str)
	return nil
}

// StringToPEMDataHookFunc for mapstructure to decode PEMData from strings.
func StringToPEMDataHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(PEMData("")) {
			return data, nil
		}

		return PEMData(data.(string)), nil
	}
}

// isValidPEM validates if the input string is a valid PEM block.
func isValidPEM(pemData string) bool {
	// Decode the PEM data
	block, _ := pem.Decode([]byte(pemData))
	return block != nil
}

// Load detects if PEMData is a file path, base64 string, or raw PEM string and loads the content.
func (p PEMData) Load(statFile StatFileFunc, readFile ReadFileFunc) ([]byte, string, error) {
	value := string(p)
	if isValidPEM(value) {
		return []byte(value), "raw pem", nil
	}
	// Check if it's base64 encoded.
	if decodedValue, err := base64.StdEncoding.DecodeString(value); err == nil {
		if isValidPEM(string(decodedValue)) {
			return decodedValue, "base64 pem", nil
		}
	}
	// Check if it's a file path by verifying if the file exists.
	if _, err := statFile(value); err == nil {
		content, err := readFile(value)
		if err != nil {
			return nil, "", fmt.Errorf("error reading file: %w", err)
		}
		if !isValidPEM(string(content)) {
			return nil, "", fmt.Errorf("file \"%s\" contains invalid PEM data", value)
		}
		return content, "pem file path", nil
	}
	return nil, "", errors.New("invalid PEM data: not a valid file path, base64-encoded PEM content, or raw PEM content")
}
