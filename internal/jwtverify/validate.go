package jwtverify

import (
	"fmt"
	"regexp/syntax"
	"strings"
)

// extractTemplatePlaceholders returns the names of all {{name}} placeholders in s.
func extractTemplatePlaceholders(s string) []string {
	var result []string
	for {
		start := strings.Index(s, "{{")
		if start == -1 {
			break
		}
		rest := s[start+2:]
		end := strings.Index(rest, "}}")
		if end == -1 {
			break
		}
		result = append(result, rest[:end])
		s = rest[end+2:]
	}
	return result
}

// findNamedGroupSubTrees parses a regex pattern and returns a map from each named
// capture group to its parsed sub-expression AST. We work with AST nodes directly
// to avoid round-tripping through String() which can introduce optimizations like
// character classes (e.g., "test1|test2" → "test[12]") that change the structure.
func findNamedGroupSubTrees(pattern string) (map[string]*syntax.Regexp, error) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]*syntax.Regexp)
	var walk func(re *syntax.Regexp)
	walk = func(re *syntax.Regexp) {
		if re.Op == syntax.OpCapture && re.Name != "" {
			if len(re.Sub) > 0 {
				groups[re.Name] = re.Sub[0]
			}
		}
		for _, sub := range re.Sub {
			walk(sub)
		}
	}
	walk(re)
	return groups, nil
}

// isLiteralTree returns true if the regexp AST can only match a finite set of fixed
// strings — no wildcards (dot), repetitions (star, plus, quest), or open-ended
// constructs. The following ops are accepted:
//   - OpLiteral, OpEmptyMatch — fixed content.
//   - OpConcat, OpCapture, OpAlternate — structural; safe if all children are safe.
//   - OpCharClass — matches exactly one rune from a finite set. The Go regexp parser
//     produces these when factoring common prefixes from alternations of literals
//     (e.g., "test1|test2" → Concat(Literal("test"), CharClass("12"))).
func isLiteralTree(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpLiteral, syntax.OpEmptyMatch, syntax.OpCharClass:
		return true
	case syntax.OpConcat, syntax.OpCapture, syntax.OpAlternate:
		for _, sub := range re.Sub {
			if !isLiteralTree(sub) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// rejectCharClassSyntax checks whether a regex pattern contains character class syntax
// that is not allowed in issuer/audience regexes when JWKS endpoint templates are used.
// This catches both bracket syntax ([...]) and shorthand classes (\d, \w, \s, \D, \W, \S)
// as well as Unicode property classes (\p, \P).
func rejectCharClassSyntax(pattern string) error {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '[':
			return fmt.Errorf("contains character class syntax which is not allowed when JWKS endpoint URL template is used")
		case '\\':
			if i+1 < len(pattern) {
				switch pattern[i+1] {
				case 'd', 'D', 'w', 'W', 's', 'S', 'p', 'P':
					return fmt.Errorf("contains character class shorthand (\\%c) which is not allowed when JWKS endpoint URL template is used", pattern[i+1])
				}
				i++ // skip escaped char
			}
		}
	}
	return nil
}

// validateJWKSEndpointSafety checks that regex named groups used as JWKS endpoint
// template placeholders are explicit alternations of fixed strings (e.g.,
// "value1|value2|value3"). This is required regardless of URL position because
// template values come from unverified JWT claims — a permissive pattern would let
// an attacker substitute an arbitrary value and redirect JWKS key fetches to a server
// they control (whether via host, subdomain on a shared cloud platform, or path on
// a shared auth service). Runtime path.Clean protection in the JWKS manager provides
// additional defense-in-depth against path traversal.
func validateJWKSEndpointSafety(endpoint, issuerRegex, audienceRegex string) error {
	placeholders := extractTemplatePlaceholders(endpoint)
	if len(placeholders) == 0 {
		return nil
	}
	placeholderSet := make(map[string]struct{}, len(placeholders))
	for _, p := range placeholders {
		placeholderSet[p] = struct{}{}
	}
	for _, regexConfig := range []struct {
		name    string
		pattern string
	}{
		{"issuer_regex", issuerRegex},
		{"audience_regex", audienceRegex},
	} {
		if regexConfig.pattern == "" {
			continue
		}
		// Reject character classes in the source pattern. The regexp parser
		// may produce OpCharClass from alternation optimization (e.g., "test1|test2"
		// → CharClass("12")), but user-written character classes must not be allowed.
		// This covers bracket syntax ([...]) and shorthand classes (\d, \w, \s, etc.).
		if err := rejectCharClassSyntax(regexConfig.pattern); err != nil {
			return fmt.Errorf(
				"%s %w — use an explicit list of allowed values "+
					"(e.g., value1|value2|value3)",
				regexConfig.name, err)
		}
		groups, err := findNamedGroupSubTrees(regexConfig.pattern)
		if err != nil {
			return fmt.Errorf("error parsing %s for JWKS safety check: %w", regexConfig.name, err)
		}
		for groupName, subTree := range groups {
			if _, ok := placeholderSet[groupName]; !ok {
				continue
			}
			if !isLiteralTree(subTree) {
				return fmt.Errorf(
					"%s named group %q is used in JWKS endpoint URL template — "+
						"only an explicit list of allowed values is permitted "+
						"(e.g., value1|value2|value3)",
					regexConfig.name, groupName)
			}
		}
	}
	return nil
}
