package pubsub

import "context"

// PubSub provides lightweight publish/subscribe on a single channel.
// Used for shared poll notifications — not for general-purpose messaging.
type PubSub interface {
	// Publish sends data to the configured channel/subject.
	Publish(ctx context.Context, data []byte) error
	// Subscribe listens on the configured channel/subject and calls handler
	// for each message. Blocks until ctx is cancelled. Reconnects automatically.
	Subscribe(ctx context.Context, handler func(data []byte)) error
	// Close releases all resources.
	Close() error
}
