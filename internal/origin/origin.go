package origin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gobwas/glob"
)

type PatternChecker struct {
	allowedOrigins []glob.Glob
}

func NewPatternChecker(allowedOrigins []string) (*PatternChecker, error) {
	var globs []glob.Glob
	for _, pattern := range allowedOrigins {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("malformed origin pattern: %w", err)
		}
		globs = append(globs, g)
	}
	return &PatternChecker{
		allowedOrigins: globs,
	}, nil
}

func (a *PatternChecker) Check(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}

	for _, pattern := range a.allowedOrigins {
		if pattern.Match(strings.ToLower(origin)) {
			return nil
		}
	}

	return fmt.Errorf("request Origin %s is not authorized", origin)
}
