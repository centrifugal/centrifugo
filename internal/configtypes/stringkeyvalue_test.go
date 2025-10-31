package configtypes

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringKeyValues_ToMap(t *testing.T) {
	t.Run("basic conversion", func(t *testing.T) {
		kvs := StringKeyValues{
			{Key: "key1", Value: "value1"},
			{Key: "key2", Value: "value2"},
			{Key: "key3", Value: "value3"},
		}

		m := kvs.ToMap()

		expected := map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}

		require.Equal(t, expected, m)
	})

	t.Run("nil receiver", func(t *testing.T) {
		var kvs *StringKeyValues
		m := kvs.ToMap()
		require.Nil(t, m)
	})

	t.Run("empty slice", func(t *testing.T) {
		kvs := StringKeyValues{}
		m := kvs.ToMap()
		require.NotNil(t, m)
		require.Empty(t, m)
	})
}

func TestStringKeyValues_Decode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envVars  map[string]string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "basic JSON map decoding",
			input:    `{"key1": "value1", "key2": "value2"}`,
			envVars:  map[string]string{},
			expected: map[string]string{"key1": "value1", "key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "basic key/value slice decoding",
			input:    `[{"key": "key1", "value": "value1"}, {"key": "key2", "value": "value2"}]`,
			envVars:  map[string]string{},
			expected: map[string]string{"key1": "value1", "key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "JSON map with environment variable expansion",
			input:    `{"header1": "${CENTRIFUGO_VAR_TEST_VAR}", "header2": "${CENTRIFUGO_VAR_ANOTHER_VAR}"}`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_TEST_VAR": "test_value", "CENTRIFUGO_VAR_ANOTHER_VAR": "another_value"},
			expected: map[string]string{"header1": "test_value", "header2": "another_value"},
			wantErr:  false,
		},
		{
			name:     "JSON map with keys containing dots",
			input:    `{"key.with.dots": "value1", "another.key": "value2"}`,
			envVars:  map[string]string{},
			expected: map[string]string{"key.with.dots": "value1", "another.key": "value2"},
			wantErr:  false,
		},
		{
			name:     "JSON map with case-sensitive keys",
			input:    `{"KeyOne": "value1", "keyone": "value2"}`,
			envVars:  map[string]string{},
			expected: map[string]string{"KeyOne": "value1", "keyone": "value2"},
			wantErr:  false,
		},
		{
			name:     "empty JSON map",
			input:    `{}`,
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "JSON map with missing environment variable",
			input:    `{"header1": "${CENTRIFUGO_VAR_MISSING}"}`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "environment variable expansion",
			input:    `[{"key": "header1", "value": "${CENTRIFUGO_VAR_TEST_VAR}"}, {"key": "header2", "value": "${CENTRIFUGO_VAR_ANOTHER_VAR}"}]`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_TEST_VAR": "test_value", "CENTRIFUGO_VAR_ANOTHER_VAR": "another_value"},
			expected: map[string]string{"header1": "test_value", "header2": "another_value"},
			wantErr:  false,
		},
		{
			name:     "mixed static and environment variables",
			input:    `[{"key": "header1", "value": "prefix-${CENTRIFUGO_VAR_TEST_VAR}-suffix"}, {"key": "header2", "value": "static_value"}]`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_TEST_VAR": "dynamic"},
			expected: map[string]string{"header1": "prefix-dynamic-suffix", "header2": "static_value"},
			wantErr:  false,
		},
		{
			name:     "missing environment variable",
			input:    `[{"key": "header1", "value": "${CENTRIFUGO_VAR_MISSING_VAR}"}, {"key": "header2", "value": "static"}]`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "multiple environment variables in one value",
			input:    `[{"key": "header1", "value": "${CENTRIFUGO_VAR_VAR1}-${CENTRIFUGO_VAR_VAR2}"}]`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_VAR1": "first", "CENTRIFUGO_VAR_VAR2": "second"},
			expected: map[string]string{"header1": "first-second"},
			wantErr:  false,
		},
		{
			name:     "missing one of multiple environment variables",
			input:    `[{"key": "header1", "value": "${CENTRIFUGO_VAR_VAR1}-${CENTRIFUGO_VAR_MISSING_VAR}"}]`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_VAR1": "first"},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "empty key/value slice",
			input:    `[]`,
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "invalid JSON",
			input:    `[{"key": "key1", "value": "value1"`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "environment variable with special characters",
			input:    `[{"key": "header1", "value": "${CENTRIFUGO_VAR_WITH_UNDERSCORES}"}, {"key": "header2", "value": "${CENTRIFUGO_VAR_123}"}]`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_WITH_UNDERSCORES": "underscore_value", "CENTRIFUGO_VAR_123": "numeric_suffix"},
			expected: map[string]string{"header1": "underscore_value", "header2": "numeric_suffix"},
			wantErr:  false,
		},
		{
			name:     "no environment variables in values",
			input:    `[{"key": "header1", "value": "plain_value"}, {"key": "header2", "value": "another_plain"}]`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_UNUSED": "unused"},
			expected: map[string]string{"header1": "plain_value", "header2": "another_plain"},
			wantErr:  false,
		},
		{
			name:     "non-CENTRIFUGO_VAR pattern ignored",
			input:    `[{"key": "header1", "value": "${INVALID_VAR}"}, {"key": "header2", "value": "static"}]`,
			envVars:  map[string]string{"INVALID_VAR": "value"},
			expected: map[string]string{"header1": "${INVALID_VAR}", "header2": "static"},
			wantErr:  false,
		},
		{
			name:     "mixed CENTRIFUGO_VAR and other patterns",
			input:    `[{"key": "header1", "value": "${CENTRIFUGO_VAR_VALID}-${INVALID_VAR}"}]`,
			envVars:  map[string]string{"CENTRIFUGO_VAR_VALID": "valid", "INVALID_VAR": "invalid"},
			expected: map[string]string{"header1": "valid-${INVALID_VAR}"},
			wantErr:  false,
		},
		{
			name:     "other prefix patterns left unchanged",
			input:    `[{"key": "header1", "value": "${OTHER_PREFIX_VAR}"}]`,
			envVars:  map[string]string{"OTHER_PREFIX_VAR": "value"},
			expected: map[string]string{"header1": "${OTHER_PREFIX_VAR}"},
			wantErr:  false,
		},
		{
			name:     "missing CENTRIFUGO_VAR environment variable",
			input:    `[{"key": "header1", "value": "${CENTRIFUGO_VAR_MISSING}"}]`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "duplicate key",
			input:    `[{"key": "key1", "value": "value1"}, {"key": "key1", "value": "value2"}]`,
			envVars:  map[string]string{},
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "empty key",
			input:    `[{"key": "", "value": "value1"}]`,
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

			var kvs StringKeyValues
			err := kvs.Decode(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("StringKeyValues.Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				actualMap := kvs.ToMap()
				if len(actualMap) != len(tt.expected) {
					t.Errorf("StringKeyValues.Decode() result length = %v, expected length %v", len(actualMap), len(tt.expected))
					return
				}

				for key, expectedValue := range tt.expected {
					if actualValue, exists := actualMap[key]; !exists {
						t.Errorf("StringKeyValues.Decode() missing key %v", key)
					} else if actualValue != expectedValue {
						t.Errorf("StringKeyValues.Decode() key %v = %v, expected %v", key, actualValue, expectedValue)
					}
				}
			}
		})
	}
}

func TestStringToStringKeyValuesHookFunc_WrongTypes(t *testing.T) {
	hook := StringToStringKeyValuesHookFunc().(func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error))

	// Test with wrong target type
	result, err := hook(
		reflect.TypeOf([]interface{}{}),     // from []interface{}
		reflect.TypeOf(map[string]string{}), // to map[string]string (not StringKeyValues)
		[]interface{}{map[string]interface{}{"key": "key1", "value": "value1"}},
	)
	require.NoError(t, err)
	require.Equal(t, []interface{}{map[string]interface{}{"key": "key1", "value": "value1"}}, result) // Should return data unchanged
}

func TestStringToStringKeyValuesHookFunc(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		envVars  map[string]string
		expected map[string]string // Use map for comparison
		wantErr  bool
	}{
		{
			name:     "basic key/value slice conversion",
			input:    []any{map[string]any{"key": "Key1", "value": "value1"}, map[string]any{"key": "Key2", "value": "value2"}},
			envVars:  map[string]string{},
			expected: map[string]string{"Key1": "value1", "Key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "environment variable expansion",
			input:    []any{map[string]any{"key": "header1", "value": "${CENTRIFUGO_VAR_TEST_VAR}"}, map[string]any{"key": "header2", "value": "${CENTRIFUGO_VAR_ANOTHER_VAR}"}},
			envVars:  map[string]string{"CENTRIFUGO_VAR_TEST_VAR": "test_value", "CENTRIFUGO_VAR_ANOTHER_VAR": "another_value"},
			expected: map[string]string{"header1": "test_value", "header2": "another_value"},
			wantErr:  false,
		},
		{
			name:     "non-CENTRIFUGO_VAR pattern ignored",
			input:    []any{map[string]any{"key": "header1", "value": "${INVALID_VAR}"}, map[string]any{"key": "header2", "value": "static"}},
			envVars:  map[string]string{"INVALID_VAR": "value"},
			expected: map[string]string{"header1": "${INVALID_VAR}", "header2": "static"},
			wantErr:  false,
		},
		{
			name:     "missing environment variable",
			input:    []any{map[string]any{"key": "header1", "value": "${CENTRIFUGO_VAR_MISSING}"}},
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  true,
		},
		{
			name:     "duplicate key",
			input:    []any{map[string]any{"key": "key1", "value": "value1"}, map[string]any{"key": "key1", "value": "value2"}},
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  true,
		},
		{
			name:     "unsupported type (map instead of slice)",
			input:    map[string]interface{}{"key1": "value1", "key2": "value2"},
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  true,
		},
		{
			name:     "non-map element in slice",
			input:    []any{"not a map"},
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  true,
		},
		{
			name:     "missing key field",
			input:    []any{map[string]any{"value": "value1"}},
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  true,
		},
		{
			name:     "missing value field",
			input:    []any{map[string]any{"key": "key1"}},
			envVars:  map[string]string{},
			expected: map[string]string{},
			wantErr:  true,
		},
	}

	hook := StringToStringKeyValuesHookFunc().(func(
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

			// Test the decode hook function
			result, err := hook(
				reflect.TypeOf([]interface{}{}),   // from []interface{}
				reflect.TypeOf(StringKeyValues{}), // to StringKeyValues
				tt.input,
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("StringToStringKeyValuesHookFunc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				resultKvs, ok := result.(StringKeyValues)
				if !ok {
					t.Errorf("StringToStringKeyValuesHookFunc() result is not StringKeyValues, got %T", result)
					return
				}

				resultMap := resultKvs.ToMap()
				if len(resultMap) != len(tt.expected) {
					t.Errorf("StringToStringKeyValuesHookFunc() result length = %v, expected length %v", len(resultMap), len(tt.expected))
					return
				}

				for key, expectedValue := range tt.expected {
					if actualValue, exists := resultMap[key]; !exists {
						t.Errorf("StringToStringKeyValuesHookFunc() missing key %v", key)
					} else if actualValue != expectedValue {
						t.Errorf("StringToStringKeyValuesHookFunc() key %v = %v, expected %v", key, actualValue, expectedValue)
					}
				}
			}
		})
	}
}
