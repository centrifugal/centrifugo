package origin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gobwas/glob"
)

type PatternChecker struct {
	allowedOrigins []*glob.Pattern
}

func NewPatternChecker(allowedOrigins []string) (*PatternChecker, error) {
	var globs []*glob.Pattern
	for _, pattern := range allowedOrigins {
		// Origin is matched in lower case (scheme and host are case-insensitive),
		// so patterns must be lower-cased too – otherwise a pattern containing
		// upper-case letters could never match.
		g, err := glob.Compile(strings.ToLower(pattern))
		if err != nil {
			return nil, fmt.Errorf("malformed origin pattern: %w", err)
		}
		globs = append(globs, g)
	}
	return &PatternChecker{
		allowedOrigins: globs,
	}, nil
}

func (a *PatternChecker) Check(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	origin = strings.ToLower(origin)
	for _, pattern := range a.allowedOrigins {
		if pattern.Match(origin) {
			return true
		}
	}

	return false
}
