package jwtverify

import (
	"regexp/syntax"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_extractTemplatePlaceholders(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want []string
	}{
		{"no placeholders", "https://example.com/.well-known/jwks.json", nil},
		{"single", "https://{{tenant}}.example.com/jwks", []string{"tenant"}},
		{"multiple", "https://{{host}}/{{path}}/jwks", []string{"host", "path"}},
		{"empty template", "https://{{}}/.well-known/jwks.json", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTemplatePlaceholders(tt.s)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_isLiteralTree(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		// --- Should pass (finite fixed strings) ---
		{"single literal", `host\.example\.com`, true},
		{"explicit alternation", `host1\.example\.com|host2\.example\.com`, true},
		{"three alternatives", `aaa|bbb|ccc`, true},
		{"single char", `a`, true},
		{"empty pattern", ``, true},

		// Common prefix factoring produces CharClass or nested Alternate.
		{"common prefix single char suffix", `test1|test2`, true},
		{"common prefix multi-option suffix", `ab|ac|ad`, true},
		{"single char alternation becomes CharClass", `a|b|c`, true},
		{"hex escape alternation", `foo\x41|foo\x42`, true},

		// Prefix factoring with one branch empty (prefix-of-prefix).
		{"one is prefix of other", `fo|foo`, true},
		{"short and long", `test|testing`, true},
		{"prefix chain", `a|ab|abc`, true},
		{"alternation with empty branch", `|foo`, true},
		{"alternation with trailing empty", `foo|`, true},

		// Escaped special chars (dots) — should be literals.
		{"escaped dots in alternation", `auth\.example\.com|auth\.other\.com`, true},
		{"escaped dots multi-segment", `auth\.us\.example\.com|auth\.eu\.example\.com|auth\.ap\.example\.com`, true},
		{"escaped dots in path-like values", `org\.acme\.production|org\.globex\.staging`, true},

		// Nested unnamed captures.
		{"nested unnamed groups", `(foo|bar)baz|(foo|bar)qux`, true},

		// --- Should fail (open-ended patterns) ---
		{"character class with plus", `[a-zA-Z0-9-]+`, false},
		{"dot plus", `.+`, false},
		{"dot star", `.*`, false},
		{"word class with plus", `\w+`, false},
		{"mixed literal and class", `host1\.example\.com|[a-z]+`, false},
		{"any char", `.`, false},
		{"question mark (optional)", `foo?`, false},
		{"star", `a*`, false},
		{"plus", `a+`, false},
		{"repeat", `a{3}`, false},
		{"dot in alternation", `foo|.`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := syntax.Parse(tt.pattern, syntax.Perl)
			require.NoError(t, err)
			require.Equal(t, tt.want, isLiteralTree(re), "pattern: %s, AST: %s", tt.pattern, re)
		})
	}
}

func Test_findNamedGroupSubTrees(t *testing.T) {
	groups, err := findNamedGroupSubTrees(`^https://(?P<tenant>[a-zA-Z0-9-]+)\.example\.com$`)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Contains(t, groups, "tenant")

	groups, err = findNamedGroupSubTrees(`^https://(.+)$`)
	require.NoError(t, err)
	require.Len(t, groups, 0) // unnamed group

	groups, err = findNamedGroupSubTrees(`^(?P<a>[a-z]+)-(?P<b>.+)$`)
	require.NoError(t, err)
	require.Len(t, groups, 2)
}

func Test_validateJWKSEndpointSafety(t *testing.T) {
	tests := []struct {
		name          string
		endpoint      string
		issuerRegex   string
		audienceRegex string
		wantErr       bool
		errContains   string
	}{
		// --- General cases ---
		{
			name:     "no placeholders - always safe",
			endpoint: "https://example.com/.well-known/jwks.json",
			wantErr:  false,
		},
		{
			name:     "placeholder but no regex - safe",
			endpoint: "https://{{tenant}}.example.com/jwks",
			wantErr:  false,
		},
		{
			name:        "named group not used as placeholder - safe even if permissive",
			endpoint:    "https://fixed-host.example.com/.well-known/jwks.json",
			issuerRegex: `^https://(?P<anything>.+)\.example\.com$`,
			wantErr:     false,
		},

		// --- Host position (entire host and subdomain) ---
		{
			name:        "entire host with explicit alternation - safe",
			endpoint:    "https://{{host}}/.well-known/jwks.json",
			issuerRegex: `^(?P<host>auth1\.example\.com|auth2\.example\.com)$`,
			wantErr:     false,
		},
		{
			name:        "entire host with single literal - safe",
			endpoint:    "https://{{host}}/.well-known/jwks.json",
			issuerRegex: `^(?P<host>auth\.example\.com)$`,
			wantErr:     false,
		},
		{
			name:        "entire host with character class - unsafe",
			endpoint:    "https://{{host}}/.well-known/jwks.json",
			issuerRegex: `^(?P<host>[a-zA-Z0-9.-]+)$`,
			wantErr:     true,
			errContains: "JWKS endpoint URL template",
		},
		{
			name:        "entire host with dot-plus - unsafe",
			endpoint:    "https://{{host}}/.well-known/jwks.json",
			issuerRegex: `^(?P<host>.+)$`,
			wantErr:     true,
			errContains: "JWKS endpoint URL template",
		},
		{
			name:        "subdomain with explicit alternation - safe",
			endpoint:    "https://{{tenant}}.example.com/.well-known/jwks.json",
			issuerRegex: `^https://(?P<tenant>acme|globex|initech)\.example\.com$`,
			wantErr:     false,
		},
		{
			name:        "subdomain with character class - unsafe",
			endpoint:    "https://{{tenant}}.example.com/.well-known/jwks.json",
			issuerRegex: `^https://(?P<tenant>[a-zA-Z0-9-]+)\.example\.com$`,
			wantErr:     true,
			errContains: "JWKS endpoint URL template",
		},
		{
			name:          "audience regex with wildcard - unsafe",
			endpoint:      "https://{{realm}}.auth.example.com/.well-known/jwks.json",
			audienceRegex: `^(?P<realm>\S+)\.auth\.example\.com$`,
			wantErr:       true,
			errContains:   "audience_regex",
		},
		{
			name:          "audience regex with alternation - safe",
			endpoint:      "https://{{realm}}.auth.example.com/.well-known/jwks.json",
			audienceRegex: `^(?P<realm>us|eu|ap)\.auth\.example\.com$`,
			wantErr:       false,
		},

		// --- Path position ---
		{
			name:        "path with explicit alternation - safe",
			endpoint:    "https://fixed-host.com/tenants/{{tenant}}/jwks.json",
			issuerRegex: `^(?P<tenant>acme|globex|initech)$`,
			wantErr:     false,
		},
		{
			name:        "path with single literal - safe",
			endpoint:    "https://fixed-host.com/tenants/{{tenant}}/jwks.json",
			issuerRegex: `^(?P<tenant>production)$`,
			wantErr:     false,
		},
		{
			name:        "path with character class - unsafe",
			endpoint:    "https://fixed-host.com/tenants/{{tenant}}/jwks.json",
			issuerRegex: `^(?P<tenant>[a-zA-Z0-9_-]+)$`,
			wantErr:     true,
			errContains: "JWKS endpoint URL template",
		},
		{
			name:        "path with dot-plus - unsafe",
			endpoint:    "https://fixed-host.com/tenants/{{tenant}}/jwks.json",
			issuerRegex: `^(?P<tenant>.+)$`,
			wantErr:     true,
			errContains: "JWKS endpoint URL template",
		},

		// --- Common prefix factoring ---
		// Go regexp parser factors common prefixes and may convert single-char
		// alternations into CharClass nodes. We work with the AST directly to
		// handle these correctly.
		{
			name:        "common prefix single char suffix - safe",
			endpoint:    "https://keycloak:443/{{realm}}/protocol/openid-connect/certs",
			issuerRegex: `https://example.com/auth/realms/(?P<realm>test1|test2)`,
			wantErr:     false,
		},
		{
			name:        "common prefix longer suffix - safe",
			endpoint:    "https://keycloak:443/{{realm}}/protocol/openid-connect/certs",
			issuerRegex: `https://example.com/auth/realms/(?P<realm>production|prod-staging)`,
			wantErr:     false,
		},
		{
			name:        "one value is prefix of another - safe",
			endpoint:    "https://keycloak:443/{{realm}}/protocol/openid-connect/certs",
			issuerRegex: `https://example.com/auth/realms/(?P<realm>test|testing)`,
			wantErr:     false,
		},
		{
			name:        "three values with common prefix - safe",
			endpoint:    "https://host/{{env}}/jwks",
			issuerRegex: `^(?P<env>dev1|dev2|dev3)$`,
			wantErr:     false,
		},
		{
			name:        "single char alternation - safe",
			endpoint:    "https://host/{{zone}}/jwks",
			issuerRegex: `^(?P<zone>a|b|c)$`,
			wantErr:     false,
		},
		{
			name:        "prefix chain - safe",
			endpoint:    "https://host/{{tier}}/jwks",
			issuerRegex: `^(?P<tier>a|ab|abc)$`,
			wantErr:     false,
		},
		{
			name:        "escaped dots in host alternation - safe",
			endpoint:    "https://{{host}}/jwks.json",
			issuerRegex: `^(?P<host>auth\.example\.com|auth\.other\.com)$`,
			wantErr:     false,
		},
		{
			name:        "escaped dots in multi-segment host alternation - safe",
			endpoint:    "https://{{host}}/protocol/openid-connect/certs",
			issuerRegex: `^https://(?P<host>auth\.us\.example\.com|auth\.eu\.example\.com|auth\.ap\.example\.com)$`,
			wantErr:     false,
		},
		{
			name:        "escaped dots in path values - safe",
			endpoint:    "https://example.com/{{realm}}/jwks.json",
			issuerRegex: `^https://example.com/(?P<realm>org\.acme\.production|org\.globex\.staging)$`,
			wantErr:     false,
		},

		// --- Patterns that must remain unsafe ---
		{
			name:        "optional suffix - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>foo?)$`,
			wantErr:     true,
		},
		{
			name:        "repetition - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>a{3})$`,
			wantErr:     true,
		},
		{
			name:        "star - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>test.*)$`,
			wantErr:     true,
		},
		{
			name:        "any char in alternation - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>foo|.)$`,
			wantErr:     true,
		},

		// --- Shorthand character classes ---
		{
			name:        "digit shorthand - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>\d+)$`,
			wantErr:     true,
			errContains: `\d`,
		},
		{
			name:        "word shorthand - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>\w+)$`,
			wantErr:     true,
			errContains: `\w`,
		},
		{
			name:        "space shorthand - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>\s)$`,
			wantErr:     true,
			errContains: `\s`,
		},
		{
			name:        "non-digit shorthand - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>\D+)$`,
			wantErr:     true,
			errContains: `\D`,
		},
		{
			name:        "unicode property - unsafe",
			endpoint:    "https://host/{{realm}}/jwks",
			issuerRegex: `^(?P<realm>\pL+)$`,
			wantErr:     true,
			errContains: `\p`,
		},
		{
			name:        "escaped dot is fine - safe",
			endpoint:    "https://{{host}}/jwks",
			issuerRegex: `^(?P<host>auth\.example\.com)$`,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWKSEndpointSafety(tt.endpoint, tt.issuerRegex, tt.audienceRegex)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_validateJWKSEndpointSafety_IntegrationWithValidate(t *testing.T) {
	// Character class rejected for any position.
	cfg := VerifierConfig{
		JWKSPublicEndpoint: "https://{{host}}/.well-known/jwks.json",
		IssuerRegex:        `^(?P<host>[a-z0-9.-]+)$`,
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "JWKS endpoint URL template")

	// Explicit alternation passes.
	cfg.IssuerRegex = `^(?P<host>auth\.example\.com|auth\.other\.com)$`
	err = cfg.Validate()
	require.NoError(t, err)

	// Path: character class also rejected.
	cfg = VerifierConfig{
		JWKSPublicEndpoint: "https://fixed-host.com/{{tenant}}/jwks.json",
		IssuerRegex:        `^(?P<tenant>[a-zA-Z0-9_-]+)$`,
	}
	err = cfg.Validate()
	require.Error(t, err)

	// Path: explicit alternation passes.
	cfg.IssuerRegex = `^(?P<tenant>acme|globex)$`
	err = cfg.Validate()
	require.NoError(t, err)

	// Insecure escape hatch: unsafe config passes validation when skip flag is set.
	cfg = VerifierConfig{
		JWKSPublicEndpoint:                  "https://{{host}}/.well-known/jwks.json",
		IssuerRegex:                         `^(?P<host>[a-z0-9.-]+)$`,
		InsecureSkipJWKSEndpointSafetyCheck: true,
	}
	err = cfg.Validate()
	require.NoError(t, err)
}
