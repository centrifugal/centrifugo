package rule

// ChannelNamespace allows to create channels with different channel options.
type ChannelNamespace struct {
	// Name is a unique namespace name.
	Name string `json:"name"`

	// Options for namespace determine channel options for channels
	// belonging to this namespace.
	NamespaceChannelOptions `mapstructure:",squash"`
}

// NamespaceChannelOptions represent channel specific configuration for namespace
// or global channel options if set on top level of configuration.
type NamespaceChannelOptions struct {
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

	// ServerSide marks all channels in namespace as server side, when on then client
	// subscribe requests to these channels will be rejected with PermissionDenied error.
	ServerSide bool `mapstructure:"server_side" json:"server_side"`

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

	// PresenceDisableForClient prevents presence to be asked by clients.
	// In this case it's available only over server-side presence call.
	PresenceDisableForClient bool `mapstructure:"presence_disable_for_client" json:"presence_disable_for_client"`

	// HistoryDisableForClient prevents history to be asked by clients.
	// In this case it's available only over server-side history call.
	// History recover mechanism if enabled will continue to work for
	// clients anyway.
	HistoryDisableForClient bool `mapstructure:"history_disable_for_client" json:"history_disable_for_client"`

	// ProxySubscribe ...
	ProxySubscribe bool `mapstructure:"proxy_subscribe" json:"proxy_subscribe"`

	// ProxyPublish ...
	ProxyPublish bool `mapstructure:"proxy_publish" json:"proxy_publish"`
}
