package configtypes

import (
	"reflect"
	"testing"
	"time"
)

type fmInner struct {
	X int `json:"x"`
}

type fmEmbedded struct {
	Z int `json:"z"`
}

func TestTypeLabel(t *testing.T) {
	cases := []struct {
		val  any
		want string
	}{
		{"", "string"},
		{true, "bool"},
		{0, "int"},
		{int32(0), "int32"},
		{int64(0), "int64"},
		{uint32(0), "uint32"},
		{float64(0), "float64"},
		{Duration(0), "Duration"}, // named numeric type kept faithful
		{PEMData(""), "PEMData"},  // named string type kept faithful
		{[]string{}, "[]string"},  // slice of scalars
		{[]Duration{}, "[]Duration"},
		{map[string]string{}, "map[string]string"},
		{fmInner{}, "fmInner"},     // struct → type name
		{&fmInner{}, "fmInner"},    // pointer to struct → element name
		{[]fmInner{}, "[]fmInner"}, // slice of structs → []name
		{time.Time{}, "time.Time"}, // time is rendered with its qualifier
	}
	for _, c := range cases {
		if got := TypeLabel(reflect.TypeOf(c.val)); got != c.want {
			t.Errorf("TypeLabel(%T) = %q, want %q", c.val, got, c.want)
		}
	}
}

func TestIsComplexType(t *testing.T) {
	complexVals := []any{fmInner{}, &fmInner{}, []fmInner{}, []*fmInner{}}
	simpleVals := []any{"", true, 0, int64(0), float64(0), Duration(0), PEMData(""), []string{}, map[string]string{}, time.Time{}}

	for _, v := range complexVals {
		if !IsComplexType(reflect.TypeOf(v)) {
			t.Errorf("IsComplexType(%T) = false, want true", v)
		}
	}
	for _, v := range simpleVals {
		if IsComplexType(reflect.TypeOf(v)) {
			t.Errorf("IsComplexType(%T) = true, want false", v)
		}
	}
}

func TestJSONKey(t *testing.T) {
	type s struct {
		A string `json:"a_field" mapstructure:"a_field"`
		B string `mapstructure:"b_field"` // no json → mapstructure fallback
		C string // no tags → field name
		D string `json:"d_field,omitempty"` // options stripped
	}
	rt := reflect.TypeOf(s{})
	want := map[string]string{"A": "a_field", "B": "b_field", "C": "C", "D": "d_field"}
	for name, w := range want {
		f, _ := rt.FieldByName(name)
		if got := JSONKey(f); got != w {
			t.Errorf("JSONKey(%s) = %q, want %q", name, got, w)
		}
	}
}

func TestIsSquash(t *testing.T) {
	type s struct {
		fmEmbedded         // anonymous → squash
		Inner      fmInner `mapstructure:",squash"` // mapstructure squash
		Inline     fmInner `yaml:",inline"`         // yaml inline
		Plain      string  `json:"plain"`           // not squash
	}
	rt := reflect.TypeOf(s{})
	squashed := []string{"fmEmbedded", "Inner", "Inline"}
	for _, name := range squashed {
		f, _ := rt.FieldByName(name)
		if !IsSquash(f) {
			t.Errorf("IsSquash(%s) = false, want true", name)
		}
	}
	if f, _ := rt.FieldByName("Plain"); IsSquash(f) {
		t.Errorf("IsSquash(Plain) = true, want false")
	}
}
