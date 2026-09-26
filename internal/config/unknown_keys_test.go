package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type unknownKeysLeaf struct {
	A int `mapstructure:"a"`
}

type unknownKeysMiddle struct {
	Leaf *unknownKeysLeaf `mapstructure:"leaf"`
}

type unknownKeysItem struct {
	Middle unknownKeysMiddle `mapstructure:"middle"`
}

type unknownKeysTop struct {
	Top    *unknownKeysLeaf  `mapstructure:"top"`
	Middle unknownKeysMiddle `mapstructure:"middle"`
	Items  []unknownKeysItem `mapstructure:"items"`
}

// A pointer-to-struct field is checked at any depth: at the top level, inside
// a nested struct and inside a slice element.
func TestFindUnknownKeysNestedPointer(t *testing.T) {
	data := map[string]any{
		"top": map[string]any{"a": 1, "odd": 4},
		"middle": map[string]any{
			"leaf": map[string]any{"a": 1, "bad": 2},
		},
		"items": []any{
			map[string]any{
				"middle": map[string]any{
					"leaf": map[string]any{"a": 1, "worse": 3},
				},
			},
		},
	}
	want := []string{"top.odd", "middle.leaf.bad", "items.[0].middle.leaf.worse"}

	// By value, as GetConfig passes the config.
	var cfg unknownKeysTop
	require.ElementsMatch(t, want, findUnknownKeys(data, cfg, ""))

	// By pointer too, and checking keys does not write into the config.
	require.ElementsMatch(t, want, findUnknownKeys(data, &cfg, ""))
	require.Nil(t, cfg.Top)
	require.Nil(t, cfg.Middle.Leaf)
}
