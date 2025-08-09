package messagefilter

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCELFilter(t *testing.T) {
	tests := []struct {
		name      string
		filter    string
		message   map[string]interface{}
		expected  bool
		shouldErr bool
	}{
		{
			name:   "simple equality",
			filter: `meta.token_address == "xxx"`,
			message: map[string]interface{}{
				"token_address": "xxx",
			},
			expected:  true,
			shouldErr: false,
		},
		{
			name:   "simple inequality",
			filter: `meta.token_address == "xxx"`,
			message: map[string]interface{}{
				"token_address": "yyy",
			},
			expected:  false,
			shouldErr: false,
		},
		{
			name:   "complex condition",
			filter: `meta.token_address == "xxx" && meta.duration == "1s"`,
			message: map[string]interface{}{
				"token_address": "xxx",
				"duration":      "1s",
			},
			expected:  true,
			shouldErr: false,
		},
		{
			name:   "complex condition false",
			filter: `meta.token_address == "xxx" && meta.duration == "1s"`,
			message: map[string]interface{}{
				"token_address": "xxx",
				"duration":      "2s",
			},
			expected:  false,
			shouldErr: false,
		},
		{
			name:   "numeric comparison",
			filter: `meta.price > 100`,
			message: map[string]interface{}{
				"price": 150.0,
			},
			expected:  true,
			shouldErr: false,
		},
		{
			name:   "string contains",
			filter: `meta.symbol.contains("BTC")`,
			message: map[string]interface{}{
				"symbol": "BTCUSDT",
			},
			expected:  true,
			shouldErr: false,
		},
		{
			name:   "invalid expression",
			filter: `meta.token_address ==`,
			message: map[string]interface{}{
				"token_address": "xxx",
			},
			expected:  false,
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filterManager := NewFilterManager()
			filter, err := filterManager.GetOrCreateFilter(tt.filter)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			messageData, err := json.Marshal(tt.message)
			if err != nil {
				t.Errorf("failed to marshal message: %v", err)
				return
			}

			result, err := filter.Evaluate(context.Background(), messageData)
			if err != nil {
				t.Errorf("unexpected evaluation error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
