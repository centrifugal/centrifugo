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
	// Publish determines if client can publish messages into channel
	// directly. This allows to use Centrifugo without backend. All
	// messages go through Centrifugo and delivered to clients. But
	// in this case you lose everything your backend code could give -
	// validation, persistence etc.
	// This option is useful mostly for demos, personal projects or
	// prototyping.
	Publish bool `json:"publish"`

	// Anonymous determines is anonymous access (with empty user ID)
	// allowed or not. In most situations your application works with
	// authorized users so every user has its own unique user ID. But
	// if you provide real-time features for public access you may need
	// unauthorized access to channels. Turn on this option and use
	// empty string as user ID.
	Anonymous bool `json:"anonymous"`

	// JoinLeave turns on(off) join/leave messages for channels.
	// When client subscribes on channel join message sent to all
	// clients in this channel. When client leaves channel (unsubscribes)
	// leave message sent.
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave"`

	// Presence turns on(off) presence information for channels.
	// Presence is a structure with clients currently subscribed on
	// channel.
	Presence bool `json:"presence"`

	// PresenceStats turns on(off) presence stats information for channels.
	// This is a short summary of presence which includes number of clients
	// subscribed on channel and number of unique users at moment.
	PresenceStats bool `mapstructure:"presence_stats" json:"presence_stats"`

	// HistorySize determines max amount of history messages for channel,
	// 0 means no history for channel. Centrifugo history has auxiliary
	// role – it can not replace your backend persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size"`

	// HistoryLifetime determines time in seconds until expiration for
	// history messages. As Centrifugo keeps history in memory (in process
	// memory or in Redis process memory) it's important to remove old
	// messages to prevent infinite memory grows.
	HistoryLifetime int `mapstructure:"history_lifetime" json:"history_lifetime"`

	// Recover enables recover mechanism for channels. This means that
	// Centrifugo will try to recover missed messages for resubscribing
	// client. This option uses messages from history and must be used
	// with reasonable HistorySize and HistoryLifetime configuration.
	HistoryRecover bool `mapstructure:"history_recover" json:"history_recover"`

	// HistoryDropInactive enables an optimization where history is
	// only saved for channels that have at least one active subscriber.
	// This can give a huge memory saving in suitable scenarios.
	HistoryDropInactive bool `mapstructure:"history_drop_inactive" json:"history_drop_inactive"`
}
