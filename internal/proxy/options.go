package proxy

// Options define some options to alter behaviour of proxy.
type Options struct {
	// ExtraHeaders is a slice of custom headers from original HTTP request to add to proxy request.
	ExtraHeaders []string
}

// Option is a type to represent various options.
type Option func(*Options)

// WithExtraHeaders allows to set ExtraHeaders.
func WithExtraHeaders(headers []string) Option {
	return func(opts *Options) {
		opts.ExtraHeaders = headers
	}
}
