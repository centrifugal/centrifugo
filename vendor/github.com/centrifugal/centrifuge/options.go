package centrifuge

// PublishOptions define some fields to alter behaviour of Publish operation.
type PublishOptions struct {
	// SkipHistory allows to prevent saving specific Publication to channel history.
	SkipHistory bool
}

// PublishOption is a type to represent various Publish options.
type PublishOption func(*PublishOptions)

// SkipHistory allows to set SkipHistory to true.
func SkipHistory() PublishOption {
	return func(opts *PublishOptions) {
		opts.SkipHistory = true
	}
}

// UnsubscribeOptions define some fields to alter behaviour of Publish operation.
type UnsubscribeOptions struct {
	// SkipHistory allows to prevent saving specific Publication to channel history.
	Resubscribe bool
}

// UnsubscribeOption is a type to represent various Unsubscribe options.
type UnsubscribeOption func(*UnsubscribeOptions)

// WithResubscribe allows to set SkipHistory to true.
func WithResubscribe() UnsubscribeOption {
	return func(opts *UnsubscribeOptions) {
		opts.Resubscribe = true
	}
}
