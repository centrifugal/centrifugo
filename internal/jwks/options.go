package jwks

import (
	"net/http"
)

// Option is used for configuring key manager.
type Option func(m *Manager)

// WithCache sets custom cache. Default is TTLCache.
func WithCache(c Cache) Option {
	return func(m *Manager) { m.cache = c }
}

// WithHTTPClient sets custom http client. By default client with 1 sec timeout used.
func WithHTTPClient(c *http.Client) Option {
	return func(m *Manager) { m.client = c }
}

// WithUseCache defines useCache option. Default is true.
func WithUseCache(flag bool) Option {
	return func(m *Manager) { m.useCache = flag }
}

// WithMaxRetries defines max retries count if request has been failed. Default is 2.
func WithMaxRetries(n uint) Option {
	return func(m *Manager) { m.retries = n }
}
