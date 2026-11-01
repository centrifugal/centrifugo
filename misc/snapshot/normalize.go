package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Normalizer replaces values that differ between runs (IDs, times, ports,
// paths, durations) with stable placeholders. One Normalizer is used per
// scenario run, so UUID placeholders are numbered by first appearance within
// that run's transcript.
type Normalizer struct {
	// literal replacements applied before the patterns, longest first.
	literals []literal
	uuids    map[string]int
	// frozen stops numbering new UUIDs: values seen first in output whose
	// order is not deterministic (sorted logs, metrics) become <uuid>.
	frozen bool
}

type literal struct {
	old, new string
}

func NewNormalizer() *Normalizer {
	return &Normalizer{uuids: map[string]int{}}
}

// AddLiteral registers an exact string replacement (temp dirs, ports,
// hostnames, unique prefixes).
func (n *Normalizer) AddLiteral(old, new string) {
	if old == "" {
		return
	}
	for _, l := range n.literals {
		if l.old == old {
			return
		}
	}
	n.literals = append(n.literals, literal{old, new})
	sort.SliceStable(n.literals, func(i, j int) bool { return len(n.literals[i].old) > len(n.literals[j].old) })
}

// Freeze stops assigning new UUID numbers.
func (n *Normalizer) Freeze() { n.frozen = true }

var (
	uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	// RFC3339 / ISO timestamps, also with fractional seconds and offsets.
	timeRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?( [+-]\d{4} [A-Z]{3,5})?`)
	// Go durations with a fractional part ("1.234ms") or sub-second units are
	// timing measurements; integral second/minute values are left alone since
	// they come from config.
	durRe        = regexp.MustCompile(`\b\d+(\.\d+)?(ns|µs|us|ms)\b|\b\d+\.\d+(s|m|h)\b|\b(\d+h)?(\d+m)?\d+\.\d+s\b`)
	multiSpaceRe = regexp.MustCompile(`  +`)
	jwtRe        = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	// Loopback addresses with ports not allocated by the tool (client side
	// ephemeral ports). Allocated ports are replaced by name before this.
	ephemeralAddrRe = regexp.MustCompile(`(127\.0\.0\.1|\[::1\]|localhost):\d{2,5}\b`)
)

func (n *Normalizer) common(s string) string {
	for _, l := range n.literals {
		s = strings.ReplaceAll(s, l.old, l.new)
	}
	s = ephemeralAddrRe.ReplaceAllString(s, "$1:<port>")
	s = jwtRe.ReplaceAllString(s, "<jwt>")
	s = uuidRe.ReplaceAllStringFunc(s, n.uuid)
	s = timeRe.ReplaceAllString(s, "<time>")
	return s
}

// String normalizes free text.
func (n *Normalizer) String(s string) string {
	s = durRe.ReplaceAllString(n.common(s), "<duration>")
	// Protobuf text format (commands and replies in log lines) randomly
	// uses one or two spaces between fields, differently per build.
	return multiSpaceRe.ReplaceAllString(s, " ")
}

// StringNoDurations normalizes text where durations are content (config
// docs, defaults) rather than measurements.
func (n *Normalizer) StringNoDurations(s string) string {
	return n.common(s)
}

func (n *Normalizer) uuid(u string) string {
	u = strings.ToLower(u)
	if i, ok := n.uuids[u]; ok {
		return fmt.Sprintf("<uuid-%d>", i)
	}
	if n.frozen {
		return "<uuid>"
	}
	i := len(n.uuids) + 1
	n.uuids[u] = i
	return fmt.Sprintf("<uuid-%d>", i)
}

// volatileKeys are JSON object keys whose values differ between runs no
// matter what the scenario does. Their values become a placeholder of the
// key name; presence of the key is kept.
var volatileKeys = map[string]bool{
	"epoch":       true,
	"time":        true,
	"timestamp":   true,
	"uptime":      true,
	"pid":         true,
	"gomaxprocs":  true,
	"runtime":     true,
	"exp":         true,
	"iat":         true,
	"expire_at":   true,
	"ttl":         true,
	"elapsed":     true,
	"duration":    true,
	"latency":     true,
	"took":        true,
	"since":       true,
	"server_time": true,
}

// Value normalizes a decoded JSON value in place and returns it.
func (n *Normalizer) Value(v any) any {
	return n.value("", v)
}

func (n *Normalizer) value(key string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys) // stable UUID numbering inside one object.
		out := make(map[string]any, len(x))
		for _, k := range keys {
			nk := n.String(k)
			if _, dup := out[nk]; dup {
				nk += "#dup"
			}
			out[nk] = n.value(k, x[k])
		}
		return out
	case []any:
		for i := range x {
			x[i] = n.value(key, x[i])
		}
		return x
	case string:
		if volatileKeys[key] && x != "" {
			return "<" + key + ">"
		}
		return n.String(x)
	case json.Number:
		if volatileKeys[key] && x.String() != "0" {
			return "<" + key + ">"
		}
		return x
	default:
		return v
	}
}

// CanonicalJSON encodes v with sorted keys and no HTML escaping.
func CanonicalJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // map keys are sorted by encoding/json.
	return strings.TrimRight(buf.String(), "\n")
}

// DecodeJSON decodes data keeping numbers as json.Number.
func DecodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return v, nil
}

// NormalizeJSONBytes decodes JSON, normalizes and canonicalizes it. Non-JSON
// input is returned as normalized text prefixed with "raw:".
func (n *Normalizer) NormalizeJSONBytes(data []byte) string {
	v, err := DecodeJSON(data)
	if err != nil {
		return "raw:" + n.String(string(data))
	}
	return CanonicalJSON(n.Value(v))
}
