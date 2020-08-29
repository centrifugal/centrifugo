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

// UnsubscribeOptions define some fields to alter behaviour of Unsubscribe operation.
type UnsubscribeOptions struct {
	// Resubscribe allows to set resubscribe protocol flag.
	Resubscribe bool
}

// UnsubscribeOption is a type to represent various Unsubscribe options.
type UnsubscribeOption func(*UnsubscribeOptions)

// WithResubscribe allows to set Resubscribe flag to true.
func WithResubscribe() UnsubscribeOption {
	return func(opts *UnsubscribeOptions) {
		opts.Resubscribe = true
	}
}

// DisconnectOptions define some fields to alter behaviour of Disconnect operation.
type DisconnectOptions struct {
	// Reconnect allows to set reconnect flag.
	Reconnect bool
}

// DisconnectOption is a type to represent various Disconnect options.
type DisconnectOption func(options *DisconnectOptions)

// WithReconnect allows to set Reconnect flag to true.
func WithReconnect() DisconnectOption {
	return func(opts *DisconnectOptions) {
		opts.Reconnect = true
	}
}

// HistoryOptions define some fields to alter History method behaviour.
type HistoryOptions struct {
	// Since used to extract publications from stream since provided StreamPosition.
	Since *StreamPosition
	// Limit number of publications to return.
	// -1 means no limit - i.e. return all publications currently in stream.
	// 0 means that caller only interested in current stream top position so Engine
	// should not return any publications in result.
	// Positive integer does what it should.
	Limit int
}

// HistoryOption is a type to represent various History options.
type HistoryOption func(options *HistoryOptions)

// WithLimit allows to set limit.
func WithLimit(limit int) HistoryOption {
	return func(opts *HistoryOptions) {
		opts.Limit = limit
	}
}

// WithNoLimit allows to not limit returned Publications amount.
// Should be used carefully inside large history streams.
func WithNoLimit() HistoryOption {
	return func(opts *HistoryOptions) {
		opts.Limit = -1
	}
}

// Since allows to set Since option.
func Since(sp StreamPosition) HistoryOption {
	return func(opts *HistoryOptions) {
		opts.Since = &sp
	}
}
