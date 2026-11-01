package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Metric values are kept only where the scenario determines them. Runtime
// and process metrics, histogram buckets and sums, summary quantiles and
// gauges measuring time or memory keep name and labels only.
var (
	metricNoValuePrefixes = []string{"go_", "process_", "promhttp_"}
	metricNoValueRe       = regexp.MustCompile(`(_bucket|_sum|_seconds|_bytes|_timestamp|_time|_uptime|_last_.*)$`)
)

type metricSample struct {
	name   string
	labels [][2]string
	value  string
}

// normalizeMetrics parses Prometheus text exposition and returns stable
// lines "name{label="value",...} value" sorted, with HELP/TYPE lines.
func normalizeMetrics(text string, n *Normalizer, filter *regexp.Regexp) []string {
	types := map[string]string{}
	type entry struct{ key, line string }
	var entries []entry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# TYPE ") {
			f := strings.Fields(line)
			if len(f) >= 4 {
				types[f[2]] = f[3]
				if filter == nil || filter.MatchString(f[2]) {
					entries = append(entries, entry{f[2] + " 1", fmt.Sprintf("# TYPE %s %s", f[2], f[3])})
				}
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "# HELP ") {
				f := strings.SplitN(line, " ", 4)
				if len(f) == 4 && (filter == nil || filter.MatchString(f[2])) {
					entries = append(entries, entry{f[2] + " 0", "# HELP " + f[2] + " " + n.StringNoDurations(f[3])})
				}
			}
			continue
		}
		s, err := parseSample(line)
		if err != nil {
			entries = append(entries, entry{"~", "unparsed: " + n.String(line)})
			continue
		}
		if filter != nil && !filter.MatchString(s.name) {
			continue
		}
		family := familyName(s.name, types)
		keep := keepMetricValue(s, family, types[family])
		var b strings.Builder
		b.WriteString(s.name)
		if len(s.labels) > 0 {
			sort.Slice(s.labels, func(i, j int) bool { return s.labels[i][0] < s.labels[j][0] })
			b.WriteByte('{')
			for i, l := range s.labels {
				if i > 0 {
					b.WriteByte(',')
				}
				val := l[1]
				if l[0] == "le" || l[0] == "quantile" {
					// keep bucket boundaries as is.
				} else {
					val = n.String(val)
				}
				fmt.Fprintf(&b, "%s=%q", l[0], val)
			}
			b.WriteByte('}')
		}
		if keep {
			b.WriteString(" " + s.value)
		} else {
			b.WriteString(" <v>")
		}
		entries = append(entries, entry{family + " 2 " + b.String(), b.String()})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.line
	}
	return out
}

func familyName(name string, types map[string]string) string {
	if _, ok := types[name]; ok {
		return name
	}
	for _, suf := range []string{"_bucket", "_sum", "_count", "_total", "_created"} {
		if strings.HasSuffix(name, suf) {
			base := strings.TrimSuffix(name, suf)
			if _, ok := types[base]; ok {
				return base
			}
		}
	}
	return name
}

func keepMetricValue(s metricSample, family, typ string) bool {
	for _, p := range metricNoValuePrefixes {
		if strings.HasPrefix(s.name, p) {
			return false
		}
	}
	for _, l := range s.labels {
		if l[0] == "quantile" {
			return false
		}
	}
	if strings.HasSuffix(s.name, "_count") && (typ == "histogram" || typ == "summary") {
		return true
	}
	if metricNoValueRe.MatchString(s.name) || metricNoValueRe.MatchString(family) {
		return false
	}
	return true
}

func parseSample(line string) (metricSample, error) {
	var s metricSample
	i := strings.IndexAny(line, "{ ")
	if i < 0 {
		return s, fmt.Errorf("no value")
	}
	s.name = line[:i]
	rest := line[i:]
	if rest[0] == '{' {
		j := 1
		for {
			for j < len(rest) && rest[j] == ' ' {
				j++
			}
			if j >= len(rest) {
				return s, fmt.Errorf("unterminated labels")
			}
			if rest[j] == '}' {
				j++
				break
			}
			eq := strings.IndexByte(rest[j:], '=')
			if eq < 0 {
				return s, fmt.Errorf("bad label")
			}
			key := rest[j : j+eq]
			j += eq + 1
			if j >= len(rest) || rest[j] != '"' {
				return s, fmt.Errorf("bad label value")
			}
			j++
			var val strings.Builder
			for j < len(rest) && rest[j] != '"' {
				if rest[j] == '\\' && j+1 < len(rest) {
					j++
					switch rest[j] {
					case 'n':
						val.WriteByte('\n')
					default:
						val.WriteByte(rest[j])
					}
				} else {
					val.WriteByte(rest[j])
				}
				j++
			}
			j++ // closing quote
			s.labels = append(s.labels, [2]string{key, val.String()})
			if j < len(rest) && rest[j] == ',' {
				j++
			}
		}
		rest = rest[j:]
	}
	f := strings.Fields(rest)
	if len(f) < 1 {
		return s, fmt.Errorf("no value")
	}
	s.value = f[0] // timestamp, if any, dropped.
	return s, nil
}
