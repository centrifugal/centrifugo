package main

import (
	"fmt"
	"sort"
	"strings"
)

// logMessage returns the "message" field of a JSON log line, or the line.
func logMessage(line string) string {
	v, err := DecodeJSON([]byte(line))
	if err != nil {
		return line
	}
	m, ok := v.(map[string]any)
	if !ok {
		return line
	}
	if s, ok := m["message"].(string); ok {
		return s
	}
	return line
}

// normalizeLogLine keeps level, message and fields (keys always; values
// normalized), dropping the timestamp.
func (n *Normalizer) normalizeLogLine(line string) string {
	v, err := DecodeJSON([]byte(line))
	if err != nil {
		return "text: " + n.String(line)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "text: " + n.String(line)
	}
	delete(m, "time")
	return CanonicalJSON(n.Value(m))
}

// sortedCounted returns normalized lines sorted, with a count prefix for
// repeated lines. Log order between goroutines is not deterministic, so the
// multiset of lines is what is compared.
func sortedCounted(lines []string) []string {
	counts := map[string]int{}
	for _, l := range lines {
		counts[l]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if counts[k] > 1 {
			out = append(out, fmt.Sprintf("%dx %s", counts[k], k))
		} else {
			out = append(out, k)
		}
	}
	return out
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
