package cli

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// withoutHiddenFields returns a copy of v (a struct or a pointer to a struct;
// the copy is of the same kind) with every field tagged `expose:"-"` left out,
// at any depth: nested structs, pointers, slices, arrays and maps of structs,
// embedded structs. The copy is of a type built at runtime: struct types that
// contain a hidden field (directly or transitively) are rebuilt without it,
// keeping the order, names and tags of all other fields, so encoding/json,
// go-toml and yaml.v3 produce the same output as for v minus the hidden keys.
// Types that marshal themselves (json.Marshaler, encoding.TextMarshaler,
// yaml.Marshaler) are kept as they are. Rebuilt types have no methods (so an
// IsZero method of a rebuilt type is not used for omitempty). Values behind
// interface-typed fields are not inspected.
func withoutHiddenFields(v any) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("can't build config type without hidden fields: %v", r)
		}
	}()
	src := reflect.ValueOf(v)
	isPtr := src.Kind() == reflect.Pointer
	if isPtr {
		if src.IsNil() {
			return v, nil
		}
		src = src.Elem()
	}
	s := newHiddenStripper()
	t := s.typeOf(src.Type())
	dst := reflect.New(t)
	s.copyValue(dst.Elem(), src)
	if isPtr {
		return dst.Interface(), nil
	}
	return dst.Elem().Interface(), nil
}

type hiddenStripper struct {
	hasHidden map[reflect.Type]bool
	stripped  map[reflect.Type]reflect.Type
	building  map[reflect.Type]bool
}

func newHiddenStripper() *hiddenStripper {
	return &hiddenStripper{
		hasHidden: map[reflect.Type]bool{},
		stripped:  map[reflect.Type]reflect.Type{},
		building:  map[reflect.Type]bool{},
	}
}

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	yamlMarshalerType = reflect.TypeFor[yaml.Marshaler]()
)

// marshalsItself reports whether t (or *t, for values marshaled through a
// pointer) controls its own encoding.
func marshalsItself(t reflect.Type) bool {
	for _, it := range []reflect.Type{jsonMarshalerType, textMarshalerType, yamlMarshalerType} {
		if t.Implements(it) || reflect.PointerTo(t).Implements(it) {
			return true
		}
	}
	return false
}

func isHidden(f reflect.StructField) bool {
	return f.Tag.Get("expose") == "-"
}

// needsRewrite reports whether t must be replaced by a rebuilt type: it is,
// or contains (through pointers, slices, arrays and maps), a struct type with
// a hidden field.
func (s *hiddenStripper) needsRewrite(t reflect.Type) bool {
	if marshalsItself(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return s.needsRewrite(t.Elem())
	case reflect.Struct:
	default:
		return false
	}
	if res, ok := s.hasHidden[t]; ok {
		return res
	}
	s.hasHidden[t] = false // Breaks cycles of recursive types.
	res := false
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() && !f.Anonymous {
			continue
		}
		if isHidden(f) || s.needsRewrite(f.Type) {
			res = true
			break
		}
	}
	s.hasHidden[t] = res
	return res
}

// typeOf returns the type to use in place of t in the copy.
func (s *hiddenStripper) typeOf(t reflect.Type) reflect.Type {
	if !s.needsRewrite(t) {
		return t
	}
	return s.rebuild(t)
}

// rebuild returns t with hidden fields removed from all struct types in it.
func (s *hiddenStripper) rebuild(t reflect.Type) reflect.Type {
	if nt, ok := s.stripped[t]; ok {
		return nt
	}
	if s.building[t] {
		panic(fmt.Sprintf("recursive type %s", t))
	}
	s.building[t] = true
	defer delete(s.building, t)
	var nt reflect.Type
	switch t.Kind() {
	case reflect.Pointer:
		nt = reflect.PointerTo(s.typeOf(t.Elem()))
	case reflect.Slice:
		nt = reflect.SliceOf(s.typeOf(t.Elem()))
	case reflect.Array:
		nt = reflect.ArrayOf(t.Len(), s.typeOf(t.Elem()))
	case reflect.Map:
		nt = reflect.MapOf(t.Key(), s.typeOf(t.Elem()))
	case reflect.Struct:
		var fields []reflect.StructField
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if isHidden(f) {
				continue
			}
			if !f.IsExported() {
				if !f.Anonymous {
					// Not seen by encoders.
					continue
				}
				panic(fmt.Sprintf("unexported embedded field %s in %s", f.Name, t))
			}
			nf := reflect.StructField{Name: f.Name, Tag: f.Tag, Anonymous: f.Anonymous}
			if f.Anonymous {
				// Embedded types are always copied into method-less types:
				// reflect.StructOf can't embed types with methods in general.
				nf.Type = s.embeddedType(f.Type)
			} else {
				nf.Type = s.typeOf(f.Type)
			}
			fields = append(fields, nf)
		}
		nt = reflect.StructOf(fields)
	default:
		nt = t
	}
	s.stripped[t] = nt
	return nt
}

func (s *hiddenStripper) embeddedType(t reflect.Type) reflect.Type {
	if marshalsItself(t) {
		return t
	}
	switch {
	case t.Kind() == reflect.Struct:
		return s.rebuild(t)
	case t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct && !marshalsItself(t.Elem()):
		return reflect.PointerTo(s.rebuild(t.Elem()))
	}
	return t
}

// copyValue copies src into dst, where dst's type is s.typeOf(src.Type()) or
// a rebuilt copy of it.
func (s *hiddenStripper) copyValue(dst, src reflect.Value) {
	if dst.Type() == src.Type() {
		dst.Set(src)
		return
	}
	switch src.Kind() {
	case reflect.Pointer:
		if src.IsNil() {
			return
		}
		p := reflect.New(dst.Type().Elem())
		s.copyValue(p.Elem(), src.Elem())
		dst.Set(p)
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeSlice(dst.Type(), src.Len(), src.Len()))
		for i := 0; i < src.Len(); i++ {
			s.copyValue(dst.Index(i), src.Index(i))
		}
	case reflect.Array:
		for i := 0; i < src.Len(); i++ {
			s.copyValue(dst.Index(i), src.Index(i))
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}
		m := reflect.MakeMapWithSize(dst.Type(), src.Len())
		iter := src.MapRange()
		for iter.Next() {
			ev := reflect.New(dst.Type().Elem()).Elem()
			s.copyValue(ev, iter.Value())
			m.SetMapIndex(iter.Key(), ev)
		}
		dst.Set(m)
	case reflect.Struct:
		// Fields of dst are the kept fields of src, in the same order.
		st := src.Type()
		j := 0
		for i := 0; i < st.NumField(); i++ {
			f := st.Field(i)
			if isHidden(f) || !f.IsExported() {
				continue
			}
			s.copyValue(dst.Field(j), src.Field(i))
			j++
		}
	default:
		dst.Set(src.Convert(dst.Type()))
	}
}
