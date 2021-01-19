package jwks

import (
	"net/http"
)

// Option is used for configuring key manager.
type Option func(m *Manager)

// WithCache sets custom cache. Default is `memory cache`.
func WithCache(c Cache) Option {
	return func(m *Manager) { m.cache = c }
}

// WithHTTPClient sets custom http client.
func WithHTTPClient(c *http.Client) Option {
	return func(m *Manager) { m.client = c }
}

// WithLookup defines cache lookup option. Default is `true`.
func WithLookup(flag bool) Option {
	return func(m *Manager) { m.lookup = flag }
}

// WithMaxRetries defines max retries count if request has been failed. Default is `5`.
func WithMaxRetries(n int) Option {
	return func(m *Manager) { m.retries = n }
}
