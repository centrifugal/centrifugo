package origin

import (
	"fmt"
	"net/http"
	"net/url"
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

	return fmt.Errorf("request Origin %q is not authorized for Host %q", origin, r.Host)
}

func CheckSameHost(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}

	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("failed to parse Origin header %q: %w", origin, err)
	}

	if strings.EqualFold(r.Host, u.Host) {
		return nil
	}

	return fmt.Errorf("request Origin %q is not authorized for Host %q", origin, r.Host)
}
