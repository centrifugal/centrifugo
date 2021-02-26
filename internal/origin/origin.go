package origin

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

func match(pattern, s string) (bool, error) {
	return filepath.Match(strings.ToLower(pattern), strings.ToLower(s))
}

type Checker struct {
	allowedOrigins []string
}

func NewChecker(allowedOrigins []string) (*Checker, error) {
	for _, pattern := range allowedOrigins {
		// check bad pattern error on start.
		if _, err := match(pattern, "_"); err != nil {
			return nil, fmt.Errorf("malformed origin pattern: %w", err)
		}
	}
	return &Checker{
		allowedOrigins: allowedOrigins,
	}, nil
}

func (a *Checker) Check(r *http.Request) error {
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

	for _, pattern := range a.allowedOrigins {
		matched, err := match(pattern, u.Host)
		if err != nil {
			return fmt.Errorf("failed to parse filepath pattern %q: %w", pattern, err)
		}
		if matched {
			return nil
		}
	}

	return fmt.Errorf("request Origin %q is not authorized for Host %q", origin, r.Host)
}
