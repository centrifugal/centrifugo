package configtypes

import (
	"reflect"
	"strings"
)

// This file hosts small reflection helpers shared by config tooling that walks
// the configuration structs — currently the `configdoc` generator and the admin
// config redaction (Pro). Keeping them here, in OSS, gives both a single source
// of truth for how config fields are classified, named, and flattened. See also
// NameForEnv in namespace.go.

// JSONKey returns the configured key for a struct field: the first segment of the
// json tag (falling back to the mapstructure tag), or the Go field name when no
// tag is present.
func JSONKey(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		tag = f.Tag.Get("mapstructure")
	}
	if tag == "" {
		return f.Name
	}
	if name := strings.Split(tag, ",")[0]; name != "" {
		return name
	}
	return f.Name
}

// IsSquash reports whether a struct field is squashed/inlined into its parent
// (an embedded struct, a mapstructure ",squash", or a yaml ",inline"). Such
// fields contribute their own fields to the parent rather than a nested object.
func IsSquash(f reflect.StructField) bool {
	return f.Anonymous ||
		strings.Contains(f.Tag.Get("mapstructure"), "squash") ||
		strings.Contains(f.Tag.Get("yaml"), "inline")
}

// IsComplexType reports whether t is object-like — a struct or a slice/array of
// structs — and therefore cannot be populated from a single environment variable
// (its keys are set separately). Scalars, slices of scalars, and maps are not
// complex (each maps to one env var). time.Time is treated as a scalar.
func IsComplexType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Ptr:
		return t.Elem().Kind() == reflect.Struct && !isTimeType(t.Elem())
	case reflect.Struct:
		return !isTimeType(t)
	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		return elem.Kind() == reflect.Struct && !isTimeType(elem)
	default:
		return false
	}
}

func isTimeType(t reflect.Type) bool {
	return t.PkgPath() == "time" && t.Name() == "Time"
}

// TypeLabel renders a human-friendly type label for a config field type. Struct
// and slice-of-struct types show their Go type name (e.g. "TLSConfig",
// "[]Consumer"); every other type shows its Go type string with the internal
// "configtypes." package qualifier stripped (e.g. "Duration", "PEMData", "int64",
// "[]string", "map[string]string"). Used by both the configdoc generator and the
// admin config UI (Pro) so the two surfaces present identical type labels.
func TypeLabel(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		if t.Elem().Kind() == reflect.Struct {
			if isTimeType(t.Elem()) {
				return stripConfigtypesQualifier(t.String())
			}
			return t.Elem().Name()
		}
	}
	if t.Kind() == reflect.Struct {
		if isTimeType(t) {
			return stripConfigtypesQualifier(t.String())
		}
		return t.Name()
	}
	if t.Kind() == reflect.Slice {
		elem := t.Elem()
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Struct {
			if isTimeType(elem) {
				return stripConfigtypesQualifier(t.String())
			}
			return "[]" + elem.Name()
		}
	}
	return stripConfigtypesQualifier(t.String())
}

func stripConfigtypesQualifier(typeStr string) string {
	return strings.ReplaceAll(typeStr, "configtypes.", "")
}
