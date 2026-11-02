package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type hiddenTestLeaf struct {
	Keep   string `json:"keep" yaml:"keep" toml:"keep"`
	Secret string `json:"secret_leaf" yaml:"secret_leaf" toml:"secret_leaf" expose:"-"`
	Num    int    `json:"num,omitempty" yaml:"num,omitempty" toml:"num,omitempty"`
}

type hiddenTestLeafView struct {
	Keep string `json:"keep" yaml:"keep" toml:"keep"`
	Num  int    `json:"num,omitempty" yaml:"num,omitempty" toml:"num,omitempty"`
}

// HiddenTestEmbedded has a method, so it can't be embedded as is into a type
// built with reflect.StructOf.
type HiddenTestEmbedded struct {
	EmbKeep   string `json:"emb_keep" yaml:"emb_keep" toml:"emb_keep"`
	EmbSecret string `json:"secret_emb" yaml:"secret_emb" toml:"secret_emb" expose:"-"`
}

func (HiddenTestEmbedded) Describe() string { return "embedded" }

type HiddenTestEmbeddedView struct {
	EmbKeep string `json:"emb_keep" yaml:"emb_keep" toml:"emb_keep"`
}

// HiddenTestPlain has no hidden fields.
type HiddenTestPlain struct {
	PlainA string `json:"plain_a" yaml:"plain_a" toml:"plain_a"`
}

func (HiddenTestPlain) Describe() string { return "plain" }

// hiddenTestMarshaler marshals itself, its fields are not inspected.
type hiddenTestMarshaler struct {
	Secret string `expose:"-"`
}

func (m hiddenTestMarshaler) MarshalJSON() ([]byte, error) { return json.Marshal("custom:" + m.Secret) }
func (m hiddenTestMarshaler) MarshalText() ([]byte, error) { return []byte("custom:" + m.Secret), nil }
func (m hiddenTestMarshaler) MarshalYAML() (any, error)    { return "custom:" + m.Secret, nil }

type hiddenTestRoot struct {
	First              string `json:"first" yaml:"first" toml:"first"`
	Secret             string `json:"secret_top" yaml:"secret_top" toml:"secret_top" expose:"-"`
	HiddenTestEmbedded `mapstructure:",squash" yaml:",inline"`
	HiddenTestPlain    `mapstructure:",squash" yaml:",inline"`
	Nested             hiddenTestLeaf              `json:"nested" yaml:"nested" toml:"nested"`
	Ptr                *hiddenTestLeaf             `json:"ptr" yaml:"ptr" toml:"ptr"`
	NilPtr             *hiddenTestLeaf             `json:"nil_ptr,omitempty" yaml:"nil_ptr,omitempty" toml:"nil_ptr,omitempty"`
	SecretPtr          *hiddenTestLeaf             `json:"secret_ptr,omitempty" yaml:"secret_ptr,omitempty" toml:"secret_ptr,omitempty" expose:"-"`
	Slice              []hiddenTestLeaf            `json:"slice" yaml:"slice" toml:"slice"`
	PtrSlice           []*hiddenTestLeaf           `json:"ptr_slice" yaml:"ptr_slice" toml:"ptr_slice"`
	NilSlice           []hiddenTestLeaf            `json:"nil_slice" yaml:"nil_slice" toml:"nil_slice"`
	Map                map[string]hiddenTestLeaf   `json:"map" yaml:"map" toml:"map"`
	PtrMap             map[string]*hiddenTestLeaf  `json:"ptr_map" yaml:"ptr_map" toml:"ptr_map"`
	Array              [2]hiddenTestLeaf           `json:"array" yaml:"array" toml:"array"`
	Marshaler          hiddenTestMarshaler         `json:"marshaler" yaml:"marshaler" toml:"marshaler"`
	Duration           configtypes.Duration        `json:"duration" yaml:"duration" toml:"duration"`
	StringMap          configtypes.MapStringString `json:"string_map" yaml:"string_map" toml:"string_map"`
	Ignored            string                      `json:"-" yaml:"-" toml:"-"`
	Last               string                      `json:"last" yaml:"last" toml:"last"`
	SecretLast         map[string]hiddenTestLeaf   `json:"secret_last" yaml:"secret_last" toml:"secret_last" expose:"-"`
	unexported         int
}

type hiddenTestRootView struct {
	First                  string `json:"first" yaml:"first" toml:"first"`
	HiddenTestEmbeddedView `mapstructure:",squash" yaml:",inline"`
	HiddenTestPlain        `mapstructure:",squash" yaml:",inline"`
	Nested                 hiddenTestLeafView             `json:"nested" yaml:"nested" toml:"nested"`
	Ptr                    *hiddenTestLeafView            `json:"ptr" yaml:"ptr" toml:"ptr"`
	NilPtr                 *hiddenTestLeafView            `json:"nil_ptr,omitempty" yaml:"nil_ptr,omitempty" toml:"nil_ptr,omitempty"`
	Slice                  []hiddenTestLeafView           `json:"slice" yaml:"slice" toml:"slice"`
	PtrSlice               []*hiddenTestLeafView          `json:"ptr_slice" yaml:"ptr_slice" toml:"ptr_slice"`
	NilSlice               []hiddenTestLeafView           `json:"nil_slice" yaml:"nil_slice" toml:"nil_slice"`
	Map                    map[string]hiddenTestLeafView  `json:"map" yaml:"map" toml:"map"`
	PtrMap                 map[string]*hiddenTestLeafView `json:"ptr_map" yaml:"ptr_map" toml:"ptr_map"`
	Array                  [2]hiddenTestLeafView          `json:"array" yaml:"array" toml:"array"`
	Marshaler              hiddenTestMarshaler            `json:"marshaler" yaml:"marshaler" toml:"marshaler"`
	Duration               configtypes.Duration           `json:"duration" yaml:"duration" toml:"duration"`
	StringMap              configtypes.MapStringString    `json:"string_map" yaml:"string_map" toml:"string_map"`
	Ignored                string                         `json:"-" yaml:"-" toml:"-"`
	Last                   string                         `json:"last" yaml:"last" toml:"last"`
}

type hiddenTestEmbeddedPtr struct {
	First               string `json:"first" yaml:"first" toml:"first"`
	*HiddenTestEmbedded `json:"emb,omitempty" yaml:"emb,omitempty" toml:"emb,omitempty"`
}

type hiddenTestEmbeddedPtrView struct {
	First                   string `json:"first" yaml:"first" toml:"first"`
	*HiddenTestEmbeddedView `json:"emb,omitempty" yaml:"emb,omitempty" toml:"emb,omitempty"`
}

type hiddenTestNoHidden struct {
	A HiddenTestPlain   `json:"a" yaml:"a" toml:"a"`
	B []HiddenTestPlain `json:"b" yaml:"b" toml:"b"`
}

func leaf(keep string, num int) hiddenTestLeaf {
	return hiddenTestLeaf{Keep: keep, Secret: "hidden-" + keep, Num: num}
}

func leafView(keep string, num int) hiddenTestLeafView {
	return hiddenTestLeafView{Keep: keep, Num: num}
}

func ptr[T any](v T) *T { return &v }

type marshalFunc func(any) ([]byte, error)

var hiddenTestFormats = map[string]marshalFunc{
	"json": func(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") },
	"toml": toml.Marshal,
	"yaml": yaml.Marshal,
}

func TestWithoutHiddenFields(t *testing.T) {
	testCases := []struct {
		name     string
		in       any
		expected any
	}{
		{
			name: "all positions",
			in: &hiddenTestRoot{
				First:              "first",
				Secret:             "hidden-top",
				HiddenTestEmbedded: HiddenTestEmbedded{EmbKeep: "emb", EmbSecret: "hidden-emb"},
				HiddenTestPlain:    HiddenTestPlain{PlainA: "plain"},
				Nested:             leaf("nested", 1),
				Ptr:                ptr(leaf("ptr", 2)),
				SecretPtr:          ptr(leaf("secret-ptr", 0)),
				Slice:              []hiddenTestLeaf{leaf("s1", 0), leaf("s2", 3)},
				PtrSlice:           []*hiddenTestLeaf{ptr(leaf("ps1", 0)), nil},
				Map:                map[string]hiddenTestLeaf{"b": leaf("mb", 0), "a": leaf("ma", 4)},
				PtrMap:             map[string]*hiddenTestLeaf{"x": ptr(leaf("pmx", 0))},
				Array:              [2]hiddenTestLeaf{leaf("a1", 0), leaf("a2", 5)},
				Marshaler:          hiddenTestMarshaler{Secret: "kept"},
				Duration:           configtypes.Duration(3 * time.Second),
				StringMap:          configtypes.MapStringString{"k": "v"},
				Ignored:            "hidden-ignored",
				Last:               "last",
				SecretLast:         map[string]hiddenTestLeaf{"z": leaf("secret-last", 0)},
				unexported:         1,
			},
			expected: &hiddenTestRootView{
				First:                  "first",
				HiddenTestEmbeddedView: HiddenTestEmbeddedView{EmbKeep: "emb"},
				HiddenTestPlain:        HiddenTestPlain{PlainA: "plain"},
				Nested:                 leafView("nested", 1),
				Ptr:                    ptr(leafView("ptr", 2)),
				Slice:                  []hiddenTestLeafView{leafView("s1", 0), leafView("s2", 3)},
				PtrSlice:               []*hiddenTestLeafView{ptr(leafView("ps1", 0)), nil},
				Map:                    map[string]hiddenTestLeafView{"b": leafView("mb", 0), "a": leafView("ma", 4)},
				PtrMap:                 map[string]*hiddenTestLeafView{"x": ptr(leafView("pmx", 0))},
				Array:                  [2]hiddenTestLeafView{leafView("a1", 0), leafView("a2", 5)},
				Marshaler:              hiddenTestMarshaler{Secret: "kept"},
				Duration:               configtypes.Duration(3 * time.Second),
				StringMap:              configtypes.MapStringString{"k": "v"},
				Last:                   "last",
			},
		},
		{
			name: "embedded pointer",
			in: &hiddenTestEmbeddedPtr{
				First:              "first",
				HiddenTestEmbedded: &HiddenTestEmbedded{EmbKeep: "emb", EmbSecret: "hidden-emb"},
			},
			expected: &hiddenTestEmbeddedPtrView{
				First:                  "first",
				HiddenTestEmbeddedView: &HiddenTestEmbeddedView{EmbKeep: "emb"},
			},
		},
		{
			name:     "embedded nil pointer",
			in:       &hiddenTestEmbeddedPtr{First: "first"},
			expected: &hiddenTestEmbeddedPtrView{First: "first"},
		},
		{
			name:     "struct value",
			in:       hiddenTestLeaf{Keep: "k", Secret: "hidden-s"},
			expected: hiddenTestLeafView{Keep: "k"},
		},
		{
			name: "no hidden fields",
			in: &hiddenTestNoHidden{
				A: HiddenTestPlain{PlainA: "a"},
				B: []HiddenTestPlain{{PlainA: "b"}},
			},
			expected: &hiddenTestNoHidden{
				A: HiddenTestPlain{PlainA: "a"},
				B: []HiddenTestPlain{{PlainA: "b"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := withoutHiddenFields(tc.in)
			require.NoError(t, err)
			for format, marshal := range hiddenTestFormats {
				t.Run(format, func(t *testing.T) {
					got, err := marshal(out)
					require.NoError(t, err)
					expected, err := marshal(tc.expected)
					require.NoError(t, err)
					require.Equal(t, string(expected), string(got))
					require.NotContains(t, string(got), "secret_")
					require.NotContains(t, string(got), "hidden-")
				})
			}
		})
	}
}

func TestWithoutHiddenFields_DoesNotModifyInput(t *testing.T) {
	in := &hiddenTestRoot{Secret: "hidden-top", Ptr: ptr(leaf("ptr", 0))}
	_, err := withoutHiddenFields(in)
	require.NoError(t, err)
	require.Equal(t, "hidden-top", in.Secret)
	require.Equal(t, "hidden-ptr", in.Ptr.Secret)
}

type hiddenTestUnexportedEmbedded struct {
	hiddenTestLeaf
}

func TestWithoutHiddenFields_Unsupported(t *testing.T) {
	_, err := withoutHiddenFields(&hiddenTestUnexportedEmbedded{})
	require.Error(t, err)
}

func defaultTestConfig(t *testing.T) config.Config {
	t.Helper()
	conf, _, err := config.GetConfig(nil, "")
	require.NoError(t, err)
	return conf
}

var hiddenTestUnmarshal = map[string]func([]byte, any) error{
	"json": json.Unmarshal,
	"toml": toml.Unmarshal,
	"yaml": yaml.Unmarshal,
}

// The output holds every option of the config, with its value, except the
// options tagged expose:"-" – whichever the build defines.
func TestWithoutHiddenFields_Config(t *testing.T) {
	conf := defaultTestConfig(t)
	// Defaults fill in the hidden RedisNats section.
	require.NotNil(t, conf.Broker.RedisNats)
	out, err := withoutHiddenFields(conf)
	require.NoError(t, err)
	require.NotNil(t, conf.Broker.RedisNats, "the config itself is not changed")
	for format, marshal := range hiddenTestFormats {
		t.Run(format, func(t *testing.T) {
			full, err := marshal(conf)
			require.NoError(t, err)
			require.Contains(t, string(full), "redisnats")
			got, err := marshal(out)
			require.NoError(t, err)
			require.NotContains(t, string(got), "redisnats")

			var expected, actual map[string]any
			require.NoError(t, hiddenTestUnmarshal[format](full, &expected))
			require.NoError(t, hiddenTestUnmarshal[format](got, &actual))
			removeHiddenKeys(reflect.TypeOf(conf), expected, format)
			require.Equal(t, expected, actual)
		})
	}
}

// removeHiddenKeys deletes from v – a value of type t, decoded from the given
// format into maps and slices – the keys of fields tagged expose:"-".
func removeHiddenKeys(t reflect.Type, v any, format string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return // encoded as a scalar, like a duration
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get(format), ",")
			if f.Anonymous && name == "" {
				removeHiddenKeys(f.Type, m, format) // inlined into the parent
				continue
			}
			if name == "" {
				name = f.Name
			}
			if f.Tag.Get("expose") == "-" {
				delete(m, name)
				continue
			}
			removeHiddenKeys(f.Type, m[name], format)
		}
	case reflect.Slice, reflect.Array:
		s, _ := v.([]any)
		for _, e := range s {
			removeHiddenKeys(t.Elem(), e, format)
		}
	case reflect.Map:
		m, _ := v.(map[string]any)
		for _, e := range m {
			removeHiddenKeys(t.Elem(), e, format)
		}
	}
}
