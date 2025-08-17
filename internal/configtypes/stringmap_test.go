package configtypes

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMapStringString_Decode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envVars  map[string]string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "basic JSON decoding",
			input:    `{"key1": "value1", "key2": "value2"}`,
			envVars:  map[string]string{},
			expected: map[string]string{"key1": "value1", "key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "environment variable expansion",
			input:    `{"header1": "${CENTRIFUGO_VAR_TEST_VAR}", "header2": "${CENTRIFUGO_VAR_ANOTHER_VAR}"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_TEST_VAR": "test_value", "CENTRIFUGO_VAR_ANOTHER_VAR": "another_value"},
			expected: map[string]string{"header1": "test_value", "header2": "another_value"},
			wantErr:  false,
		},
		{
			name:     "mixed static and environment variables",
			input:    `{"header1": "prefix-${CENTRIFUGO_VAR_TEST_VAR}-suffix", "header2": "static_value"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_TEST_VAR": "dynamic"},
			expected: map[string]string{"header1": "prefix-dynamic-suffix", "header2": "static_value"},
			wantErr:  false,
		},
		{
			name:     "missing environment variable",
			input:    `{"header1": "${CENTRIFUGO_VAR_MISSING_VAR}", "header2": "static"}`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "multiple environment variables in one value",
			input:    `{"header1": "${CENTRIFUGO_VAR_VAR1}-${CENTRIFUGO_VAR_VAR2}"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_VAR1": "first", "CENTRIFUGO_VAR_VAR2": "second"},
			expected: map[string]string{"header1": "first-second"},
			wantErr:  false,
		},
		{
			name:     "missing one of multiple environment variables",
			input:    `{"header1": "${CENTRIFUGO_VAR_VAR1}-${CENTRIFUGO_VAR_MISSING_VAR}"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_VAR1": "first"},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "empty JSON object",
			input:    `{}`,
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "invalid JSON",
			input:    `{"key1": "value1"`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "environment variable with special characters",
			input:    `{"header1": "${CENTRIFUGO_VAR_WITH_UNDERSCORES}", "header2": "${CENTRIFUGO_VAR_123}"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_WITH_UNDERSCORES": "underscore_value", "CENTRIFUGO_VAR_123": "numeric_suffix"},
			expected: map[string]string{"header1": "underscore_value", "header2": "numeric_suffix"},
			wantErr:  false,
		},
		{
			name:     "no environment variables in values",
			input:    `{"header1": "plain_value", "header2": "another_plain"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_UNUSED": "unused"},
			expected: map[string]string{"header1": "plain_value", "header2": "another_plain"},
			wantErr:  false,
		},
		{
			name:     "non-CENTRIFUGO_VAR pattern ignored",
			input:    `{"header1": "${INVALID_VAR}", "header2": "static"}`,
			envVars:  map[string]string{"INVALID_VAR": "value"},
			expected: map[string]string{"header1": "${INVALID_VAR}", "header2": "static"},
			wantErr:  false,
		},
		{
			name:     "mixed CENTRIFUGO_VAR and other patterns",
			input:    `{"header1": "${CENTRIFUGO_VAR_VALID}-${INVALID_VAR}"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_VALID": "valid", "INVALID_VAR": "invalid"},
			expected: map[string]string{"header1": "valid-${INVALID_VAR}"},
			wantErr:  false,
		},
		{
			name:     "other prefix patterns left unchanged",
			input:    `{"header1": "${OTHER_PREFIX_VAR}"}`,
			envVars:  map[string]string{"OTHER_PREFIX_VAR": "value"},
			expected: map[string]string{"header1": "${OTHER_PREFIX_VAR}"},
			wantErr:  false,
		},
		{
			name:     "missing CENTRIFUGO_VAR environment variable",
			input:    `{"header1": "${CENTRIFUGO_VAR_MISSING}"}`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			originalEnvVars := make(map[string]string)
			for key, value := range tt.envVars {
				originalEnvVars[key] = os.Getenv(key)
				err := os.Setenv(key, value)
				require.NoError(t, err)
			}

			// Clean up environment variables after test
			defer func() {
				for key := range tt.envVars {
					if originalValue, existed := originalEnvVars[key]; existed {
						err := os.Setenv(key, originalValue)
						require.NoError(t, err)
					} else {
						err := os.Unsetenv(key)
						require.NoError(t, err)
					}
				}
			}()

			var m MapStringString
			err := m.Decode(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("MapStringString.Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(m) != len(tt.expected) {
					t.Errorf("MapStringString.Decode() result length = %v, expected length %v", len(m), len(tt.expected))
					return
				}

				for key, expectedValue := range tt.expected {
					if actualValue, exists := m[key]; !exists {
						t.Errorf("MapStringString.Decode() missing key %v", key)
					} else if actualValue != expectedValue {
						t.Errorf("MapStringString.Decode() key %v = %v, expected %v", key, actualValue, expectedValue)
					}
				}
			}
		})
	}
}

func TestStringToMapStringStringHookFunc_WrongTypes(t *testing.T) {
	hook := StringToMapStringStringHookFunc().(func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error))

	// Test with non-map source type
	result, err := hook(
		reflect.TypeOf(123),               // from int
		reflect.TypeOf(MapStringString{}), // to MapStringString
		123,
	)
	require.NoError(t, err)
	require.Equal(t, 123, result) // Should return data unchanged

	// Test with wrong target type
	result, err = hook(
		reflect.TypeOf(map[string]interface{}{}), // from map[string]interface{}
		reflect.TypeOf(map[string]string{}),      // to map[string]string (not MapStringString)
		map[string]interface{}{"key": "value"},
	)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"key": "value"}, result) // Should return data unchanged
}

func TestStringToMapStringStringHookFunc(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		envVars  map[string]string
		expected MapStringString
		wantErr  bool
	}{
		{
			name:     "basic map conversion",
			input:    map[string]interface{}{"key1": "value1", "key2": "value2"},
			envVars:  map[string]string{},
			expected: MapStringString{"key1": "value1", "key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "environment variable expansion",
			input:    map[string]interface{}{"header1": "${CENTRIFUGO_VAR_TEST_VAR}", "header2": "${CENTRIFUGO_VAR_ANOTHER_VAR}"},
			envVars:  map[string]string{"CENTRIFUGO_VAR_TEST_VAR": "test_value", "CENTRIFUGO_VAR_ANOTHER_VAR": "another_value"},
			expected: MapStringString{"header1": "test_value", "header2": "another_value"},
			wantErr:  false,
		},
		{
			name:     "non-CENTRIFUGO_VAR pattern ignored",
			input:    map[string]interface{}{"header1": "${INVALID_VAR}", "header2": "static"},
			envVars:  map[string]string{"INVALID_VAR": "value"},
			expected: MapStringString{"header1": "${INVALID_VAR}", "header2": "static"},
			wantErr:  false,
		},
		{
			name:     "missing environment variable",
			input:    map[string]interface{}{"header1": "${CENTRIFUGO_VAR_MISSING}"},
			envVars:  map[string]string{},
			expected: MapStringString{},
			wantErr:  true,
		},
		{
			name:    "non-string value in map",
			input:   map[string]interface{}{"key1": "value1", "key2": 123},
			envVars: map[string]string{},
			wantErr: true,
		},
	}

	hook := StringToMapStringStringHookFunc().(func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			originalEnvVars := make(map[string]string)
			for key, value := range tt.envVars {
				originalEnvVars[key] = os.Getenv(key)
				err := os.Setenv(key, value)
				require.NoError(t, err)
			}

			// Clean up environment variables after test
			defer func() {
				for key := range tt.envVars {
					if originalValue, existed := originalEnvVars[key]; existed {
						err := os.Setenv(key, originalValue)
						require.NoError(t, err)
					} else {
						err := os.Unsetenv(key)
						require.NoError(t, err)
					}
				}
			}()

			// Test the decode hook function with map input
			result, err := hook(
				reflect.TypeOf(map[string]interface{}{}), // from map[string]interface{}
				reflect.TypeOf(MapStringString{}),        // to MapStringString
				tt.input,
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("StringToMapStringStringHookFunc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.name == "non-string value in map" {
					// Should return original data unchanged
					require.Equal(t, tt.input, result)
				} else {
					resultMap, ok := result.(MapStringString)
					if !ok {
						t.Errorf("StringToMapStringStringHookFunc() result is not MapStringString, got %T", result)
						return
					}

					if len(resultMap) != len(tt.expected) {
						t.Errorf("StringToMapStringStringHookFunc() result length = %v, expected length %v", len(resultMap), len(tt.expected))
						return
					}

					for key, expectedValue := range tt.expected {
						if actualValue, exists := resultMap[key]; !exists {
							t.Errorf("StringToMapStringStringHookFunc() missing key %v", key)
						} else if actualValue != expectedValue {
							t.Errorf("StringToMapStringStringHookFunc() key %v = %v, expected %v", key, actualValue, expectedValue)
						}
					}
				}
			}
		})
	}
}
