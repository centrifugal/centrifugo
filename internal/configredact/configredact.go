// Package configredact walks an effective Centrifugo configuration via
// reflection and produces a redacted node tree. It is the one place that
// decides which config values may be shown.
//
// Policy:
//   - Only string-kind leaves are governed. Non-strings are always shown.
//   - A string is a SECRET by default (safe-by-default); the `expose` struct
//     tag opts specific fields into being shown: `expose:"full"` (shown as-is)
//     or `expose:"url"` (URL/DSN with credentials masked). `expose:"-"` hides
//     the field from the tree entirely.
//   - Secrets reveal presence (set/empty) only. With Options.Peek they also
//     reveal a short partial peek, never the full value. The masking policy
//     lives in one place (the constants below) so it is easy to tune later.
//
// Field documentation is read at runtime from the `doc` struct tag (user-facing
// text), the `default` tag, and reflection — there is no generated artifact.
//
// Field classification helpers shared with the `configdoc` generator (object vs
// leaf detection, json key, squash flattening, env-name normalisation) live in
// the configtypes package so both tools apply the same rules.
package configredact

import (
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

// Masking policy — the single place to tune how much of a value is revealed.
const (
	peekEdge   = 2  // secret with peek: runes shown at each end of a long value
	peekMinLen = 12 // secret with peek: values shorter than this are masked fully
	urlShow    = 3  // url with peek: leading runes shown for each credential / query value
	urlMinLen  = 6  // url with peek: values shorter than this are masked fully
	maskFull   = "••••"
	maskShort  = "•••"
)

const (
	exposeFull = "full"
	exposeURL  = "url"
	exposeHide = "-"
)

// Doc carries user-facing documentation for a field (from the `doc`/`default`
// tags and reflection). Description may contain <<code>> markers meant to be
// rendered as inline code.
type Doc struct {
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Default     any    `json:"default,omitempty"`
	Env         string `json:"env,omitempty"`
}

// Node is one field in the redacted config tree.
type Node struct {
	Key        string  `json:"key"`
	Kind       string  `json:"kind"` // object|array|string|number|bool|null|secret|url
	Value      any     `json:"value,omitempty"`
	Display    *string `json:"display,omitempty"` // secret/url: masked display
	Set        *bool   `json:"set,omitempty"`     // secret/url: presence
	Children   []Node  `json:"children,omitempty"`
	Items      []Node  `json:"items,omitempty"`
	Overridden bool    `json:"overridden,omitempty"` // value differs from default
	Doc        *Doc    `json:"doc,omitempty"`
}

// Response is the redacted config tree.
type Response struct {
	Fields []Node `json:"fields"`
}

// Options tune the output. The zero value shows no part of hidden values and
// includes field documentation.
type Options struct {
	// Peek reveals a short partial peek of hidden values: both ends of a long
	// secret, the first runes of URL credentials and query values. Without it
	// secrets show presence only and URL credentials are masked fully.
	Peek bool
	// OmitDocs leaves Node.Doc unset.
	OmitDocs bool
}

// Build walks the effective config and returns the redacted node tree. defaults
// is the same config type with only defaults applied (used to compute the
// `overridden` flag).
func Build(effective, defaults any, opts Options) Response {
	return Response{Fields: structChildren(opts, reflect.ValueOf(effective), reflect.ValueOf(defaults), "")}
}

// Value redacts a single string under the same policy as a config field tagged
// with the given expose value: "full" returns s as is, "url" masks URL
// credentials, "-" and anything else (including "") hide s. Used for values
// outside the config tree, such as headers, token claims and log fields. An
// empty s is returned as is; a hidden non-empty s becomes a mask (with a
// partial peek when opts.Peek is set, except for "-", which never peeks).
func Value(s, expose string, opts Options) string {
	if s == "" {
		return ""
	}
	switch expose {
	case exposeFull:
		return s
	case exposeURL:
		_, d := maskURL(s, opts.Peek)
		return d
	case exposeHide:
		return maskFull
	}
	if opts.Peek {
		return maskSecret(s)
	}
	return maskFull
}

func maskSecret(s string) string {
	r := []rune(s)
	if len(r) < peekMinLen {
		return maskFull
	}
	return string(r[:peekEdge]) + "…" + string(r[len(r)-peekEdge:])
}

// showPrefix reveals only the first few runes of a credential/query value, or
// none of them without peek.
func showPrefix(s string, peek bool) string {
	if !peek {
		return maskShort
	}
	r := []rune(s)
	if len(r) < urlMinLen {
		return maskShort
	}
	return string(r[:urlShow]) + "…"
}

// maskURL masks credentials in a URL/DSN: the userinfo password, a username
// without a password (token auth, as in nats://token@host), EVERY query
// parameter value and bare query items are reduced to their first few runes
// with peek, masked fully without it. If the string does not parse as a URL, or
// its path looks like it holds another URL or userinfo (several URLs joined in
// one value), it fails closed — returned as a plain masked secret.
func maskURL(s string, peek bool) (kind, display string) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" || strings.Contains(u.Path, "@") || strings.Contains(u.Path, "://") {
		if peek {
			return "secret", maskSecret(s)
		}
		return "secret", maskFull
	}
	var b strings.Builder
	b.WriteString(u.Scheme + "://")
	if u.User != nil {
		name := u.User.Username()
		if pw, ok := u.User.Password(); ok {
			b.WriteString(name + ":" + showPrefix(pw, peek) + "@")
		} else if name != "" {
			b.WriteString(showPrefix(name, peek) + "@")
		}
	}
	b.WriteString(u.Host)
	b.WriteString(u.Path)
	if u.RawQuery != "" {
		b.WriteString("?")
		for i, p := range strings.Split(u.RawQuery, "&") {
			if i > 0 {
				b.WriteString("&")
			}
			if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
				b.WriteString(kv[0] + "=" + showPrefix(kv[1], peek))
			} else if p != "" {
				b.WriteString(showPrefix(p, peek))
			}
		}
	}
	return "url", b.String()
}

func overridden(val, def reflect.Value, defaultTag string) bool {
	if !val.IsValid() || !val.CanInterface() {
		return false
	}
	if def.IsValid() && def.CanInterface() {
		return !reflect.DeepEqual(val.Interface(), def.Interface())
	}
	// No default counterpart in the defaults config — this happens for fields
	// inside slice/map elements (namespaces, proxies, consumers, …) that the
	// defaults-only config does not materialize. The defaults for such fields are
	// applied per-element from their `default` struct tag, so the tag is the only
	// source of truth here: compare against it when present.
	if defaultTag != "" {
		if matches, ok := matchesDefaultTag(val, defaultTag); ok {
			return !matches
		}
	}
	// No tag (or an un-parseable one) — treat the type's zero value as the
	// default, so unset fields like `false` booleans aren't flagged as overridden.
	return !reflect.DeepEqual(val.Interface(), reflect.Zero(val.Type()).Interface())
}

// matchesDefaultTag reports whether val equals the value its `default` struct tag
// would decode to. The second return is false when the tag can't be interpreted
// for val's kind, signalling the caller to fall back to a zero-value comparison.
func matchesDefaultTag(val reflect.Value, tag string) (bool, bool) {
	switch val.Kind() {
	case reflect.String:
		return val.String() == tag, true
	case reflect.Bool:
		b, err := strconv.ParseBool(tag)
		if err != nil {
			return false, false
		}
		return val.Bool() == b, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Named duration types (e.g. configtypes.Duration) carry tags like "1s".
		if _, ok := val.Interface().(fmt.Stringer); ok {
			if d, err := time.ParseDuration(tag); err == nil {
				return val.Int() == int64(d), true
			}
		}
		n, err := strconv.ParseInt(tag, 0, 64)
		if err != nil {
			return false, false
		}
		return val.Int() == n, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(tag, 0, 64)
		if err != nil {
			return false, false
		}
		return val.Uint() == n, true
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(tag, 64)
		if err != nil {
			return false, false
		}
		return val.Float() == f, true
	}
	return false, false
}

func derefIfPtr(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// stringerScalar renders named numeric types that implement fmt.Stringer
// (e.g. Duration → "720h0m0s") as their text form.
func stringerScalar(val reflect.Value) (string, bool) {
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		if val.CanInterface() {
			if s, ok := val.Interface().(fmt.Stringer); ok {
				return s.String(), true
			}
		}
	}
	return "", false
}

func node(opts Options, key, path, expose, defaultTag string, val, def reflect.Value) Node {
	for val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface {
		if val.IsNil() {
			return Node{Key: key, Kind: "null", Overridden: overridden(val, def, defaultTag)}
		}
		val = val.Elem()
	}
	def = derefIfPtr(def)

	if str, ok := stringerScalar(val); ok {
		return Node{Key: key, Kind: "string", Value: str, Overridden: overridden(val, def, defaultTag)}
	}

	switch val.Kind() {
	case reflect.Struct:
		return Node{Key: key, Kind: "object", Children: structChildren(opts, val, def, path)}
	case reflect.Slice, reflect.Array:
		return Node{Key: key, Kind: "array", Items: sliceItems(opts, expose, defaultTag, val, def, path)}
	case reflect.Map:
		return Node{Key: key, Kind: "object", Children: mapChildren(opts, expose, val, def, path)}
	case reflect.String:
		return stringNode(opts, key, expose, defaultTag, val, def)
	case reflect.Bool:
		return Node{Key: key, Kind: "bool", Value: val.Bool(), Overridden: overridden(val, def, defaultTag)}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return Node{Key: key, Kind: "number", Value: val.Interface(), Overridden: overridden(val, def, defaultTag)}
	default:
		s := fmt.Sprintf("%v", val.Interface())
		set := s != ""
		n := Node{Key: key, Kind: "secret", Set: &set, Overridden: overridden(val, def, defaultTag)}
		if set && opts.Peek {
			d := maskSecret(s)
			n.Display = &d
		}
		return n
	}
}

func stringNode(opts Options, key, expose, defaultTag string, val, def reflect.Value) Node {
	s := val.String()
	ov := overridden(val, def, defaultTag)
	if expose == exposeFull {
		return Node{Key: key, Kind: "string", Value: s, Overridden: ov}
	}
	set := s != ""
	kind := "secret"
	if expose == exposeURL {
		kind = "url"
	}
	n := Node{Key: key, Kind: kind, Set: &set, Overridden: ov}
	if set {
		if expose == exposeURL {
			k, d := maskURL(s, opts.Peek)
			n.Kind = k
			// An unparsable URL fails closed to a secret: no display without peek.
			if k == "url" || opts.Peek {
				n.Display = &d
			}
		} else if opts.Peek {
			d := maskSecret(s)
			n.Display = &d
		}
	}
	return n
}

// envSuppressed is a path sentinel marking a subtree whose fields have no
// environment variable — inside a map, or inside an array element that is not a
// named collection. It propagates through joinEnvPath so the whole subtree is
// excluded from env hints.
const envSuppressed = "\x00"

// joinEnvPath appends key to a dotted env path, preserving the suppression sentinel.
func joinEnvPath(parent, key string) string {
	if parent == envSuppressed {
		return envSuppressed
	}
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// elementEnvName returns the env-var segment for an array element, derived from its
// `name` field. Only the named collections (channel/rpc namespaces, proxies,
// consumers and other arrays of elements with a `name` field) support
// per-element env vars of the form CENTRIFUGO_<ARRAY>_<NAME>_<FIELD>; an element
// without a non-empty `name` field reports false, suppressing env for its fields.
func elementEnvName(v reflect.Value) (string, bool) {
	v = derefIfPtr(v)
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return "", false
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" || configtypes.JSONKey(f) != "name" {
			continue
		}
		fv := v.Field(i)
		if fv.Kind() != reflect.String || fv.String() == "" {
			return "", false
		}
		return configtypes.NameForEnv(fv.String()), true
	}
	return "", false
}

// buildDoc assembles a field's documentation from its tags + type at runtime.
func buildDoc(f reflect.StructField, path string) *Doc {
	d := &Doc{
		Description: f.Tag.Get("doc"),
		Type:        configtypes.TypeLabel(f.Type),
	}
	// Env vars target individual leaf keys (plus whole scalar slices and maps),
	// never objects: Centrifugo can't populate a struct or a list of objects from
	// a single environment variable — their keys are set separately. They are also
	// only derivable where path is known (not under a map or a non-named array
	// element). This mirrors the env-var rules of the `centrifugo configdoc` tool.
	if path != envSuppressed && !configtypes.IsComplexType(f.Type) {
		d.Env = "CENTRIFUGO_" + strings.ToUpper(strings.ReplaceAll(path, ".", "_"))
	}
	if def := f.Tag.Get("default"); def != "" {
		d.Default = def
	}
	return d
}

func structChildren(opts Options, val, def reflect.Value, path string) []Node {
	val = derefIfPtr(val)
	def = derefIfPtr(def)
	if !val.IsValid() || val.Kind() != reflect.Struct {
		return nil
	}
	var out []Node
	t := val.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		fv := val.Field(i)
		var fd reflect.Value
		if def.IsValid() && def.Kind() == reflect.Struct {
			fd = def.Field(i)
		}
		if configtypes.IsSquash(f) { // flatten embedded/squashed structs (ChannelOptions, …)
			out = append(out, structChildren(opts, fv, fd, path)...)
			continue
		}
		expose := f.Tag.Get("expose")
		if expose == exposeHide {
			continue
		}
		key := configtypes.JSONKey(f)
		if key == "-" {
			continue
		}
		childPath := joinEnvPath(path, key)
		n := node(opts, key, childPath, expose, f.Tag.Get("default"), fv, fd)
		if !opts.OmitDocs {
			n.Doc = buildDoc(f, childPath)
		}
		out = append(out, n)
	}
	return out
}

func sliceItems(opts Options, expose, defaultTag string, val, def reflect.Value, path string) []Node {
	// A slice's `default` tag encodes the whole-slice default as space-separated
	// element values (envconfig's SliceSep). When the slice has no counterpart in
	// the defaults config (it lives inside a slice/map element), fall back to this
	// tag, matching element i against the i-th encoded part.
	var defParts []string
	if defaultTag != "" {
		defParts = strings.Split(defaultTag, " ")
	}
	out := make([]Node, 0, val.Len())
	for i := 0; i < val.Len(); i++ {
		var di reflect.Value
		if def.IsValid() && (def.Kind() == reflect.Slice || def.Kind() == reflect.Array) && i < def.Len() {
			di = def.Index(i)
		}
		var itemTag string
		if !di.IsValid() && i < len(defParts) {
			itemTag = defParts[i]
		}
		// Fields inside an array element are env-settable only for the named
		// collections, where the element's name is part of the var
		// (CENTRIFUGO_<ARRAY>_<NAME>_<FIELD>). Other elements suppress env.
		itemPath := envSuppressed
		if name, ok := elementEnvName(val.Index(i)); ok {
			itemPath = joinEnvPath(path, name)
		}
		out = append(out, node(opts, strconv.Itoa(i), itemPath, expose, itemTag, val.Index(i), di))
	}
	return out
}

func mapChildren(opts Options, expose string, val, def reflect.Value, _ string) []Node {
	keys := val.MapKeys()
	sort.Slice(keys, func(a, b int) bool {
		return fmt.Sprint(keys[a].Interface()) < fmt.Sprint(keys[b].Interface())
	})
	out := make([]Node, 0, len(keys))
	for _, k := range keys {
		var dv reflect.Value
		if def.IsValid() && def.Kind() == reflect.Map {
			dv = def.MapIndex(k)
		}
		// A map is set via a single env var on the map field itself; its arbitrary,
		// user-defined keys have no individual env vars, so suppress env below here.
		out = append(out, node(opts, fmt.Sprint(k.Interface()), envSuppressed, expose, "", val.MapIndex(k), dv))
	}
	return out
}
