package centrifuge

// ChannelNamespace allows to create channels with different channel options.
type ChannelNamespace struct {
	// Name is a unique namespace name.
	Name string `json:"name"`

	// Options for namespace determine channel options for channels
	// belonging to this namespace.
	ChannelOptions `mapstructure:",squash"`
}

// ChannelOptions represent channel specific configuration for namespace
// or global channel options if set on top level of configuration.
type ChannelOptions struct {
	// Publish enables possibility for clients to publish messages into channels.
	// Once enabled client can publish into channel and that publication will be
	// broadcasted to all current channel subscribers. You can control publishing
	// on server-side setting On().Publish callback to client connection.
	Publish bool `json:"publish"`

	// SubscribeToPublish turns on an automatic check that client subscribed
	// on channel before allow it to publish into that channel.
	SubscribeToPublish bool `mapstructure:"subscribe_to_publish" json:"subscribe_to_publish"`

	// Anonymous enables anonymous access (with empty user ID) to channel.
	// In most situations your application works with authenticated users so
	// every user has its own unique user ID. But if you provide real-time
	// features for public access you may need unauthenticated access to channels.
	// Turn on this option and use empty string as user ID.
	Anonymous bool `json:"anonymous"`

	// JoinLeave turns on join/leave messages for channels.
	// When client subscribes on channel join message sent to all
	// clients in this channel. When client leaves channel (unsubscribes)
	// leave message sent. This option does not fit well for channels with
	// many subscribers because every subscribe/unsubscribe event results
	// into join/leave event broadcast to all other active subscribers.
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave"`

	// Presence turns on presence information for channels.
	// Presence is a structure with clients currently subscribed on channel.
	Presence bool `json:"presence"`

	// HistorySize determines max amount of history messages for channel,
	// 0 means no history for channel. Centrifugo history has auxiliary
	// role – it can not replace your backend persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size"`

	// HistoryLifetime determines time in seconds until expiration for
	// history messages. As Centrifuge-based server keeps history in memory
	// (for example in process memory or in Redis process memory) it's
	// important to remove old messages to prevent infinite memory grows.
	HistoryLifetime int `mapstructure:"history_lifetime" json:"history_lifetime"`

	// Recover enables recover mechanism for channels. This means that
	// server will try to recover missed messages for resubscribing
	// client. This option uses publications from history and must be used
	// with reasonable HistorySize and HistoryLifetime configuration.
	HistoryRecover bool `mapstructure:"history_recover" json:"history_recover"`
}
