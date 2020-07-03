package centrifuge

// ChannelOptionsFunc is a function that Centrifuge will call every time
// it needs to get ChannelOptions for a channel. Calls to this func can
// happen concurrently – so you need to synchronize code inside function
// implementation. The obvious advice regarding to ChannelOptions - only
// turn on various ChannelOptions features for channels where feature is
// required. For example – if you don't want collecting Presence information
// for channel then do not return options with Presence on – since every
// enabled option requires additional work on server and affects overall
// server performance.
type ChannelOptionsFunc func(channel string) (ChannelOptions, error)

// ChannelOptions represent channel configuration. It contains several
// options to tune features for channel – for example tell Centrifuge to
// maintain presence information inside channel, or configure a window of
// Publication messages that will be kept for a channel.
type ChannelOptions struct {
	// Presence turns on presence information for channel. Presence has
	// information about all clients currently subscribed to a channel.
	Presence bool `mapstructure:"presence" json:"presence"`

	// JoinLeave turns on join/leave messages for a channel.
	// When client subscribes on a channel join message sent to all
	// subscribers in this channel (including current client). When client
	// leaves channel (unsubscribes) leave message sent. This option does
	// not fit well for channels with many subscribers because every
	// subscribe/unsubscribe event results into join/leave event broadcast
	// to all other active subscribers thus overloads server with tons of
	// messages. Use accurately for channels with small number of active
	// subscribers.
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave"`

	// HistorySize determines max amount of history messages for a channel,
	// Zero value means no history for channel. Centrifuge history has an
	// auxiliary role with current Engines – it can not replace your backend
	// persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size"`

	// HistoryLifetime determines time in seconds until expiration happens
	// for history cache. As Centrifuge-based server maintains a window of
	// messages in memory (or in Redis with Redis engine), to prevent infinite
	// memory grows it's important to remove history for inactive channels.
	HistoryLifetime int `mapstructure:"history_lifetime" json:"history_lifetime"`

	// HistoryRecover enables recovery mechanism for channels. This means that
	// server will try to recover missed messages for resubscribing client.
	// This option uses publications from history and must be used with reasonable
	// HistorySize and HistoryLifetime configuration.
	HistoryRecover bool `mapstructure:"history_recover" json:"history_recover"`
}
