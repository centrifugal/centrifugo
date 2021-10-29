package rule

import "github.com/centrifugal/centrifugo/v3/internal/tools"

// ChannelNamespace allows to create channels with different channel options.
type ChannelNamespace struct {
	// Name is a unique namespace name.
	Name string `mapstructure:"name" json:"name"`

	// Options for namespace determine channel options for channels
	// belonging to this namespace.
	ChannelOptions `mapstructure:",squash"`
}

// ChannelOptions represent channel specific configuration for namespace
// or global channel options if set on top level of configuration.
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

	// HistoryTTL is a time to live for history cache. Server maintains a window of
	// messages in memory (or in Redis with Redis engine), to prevent infinite memory
	// grows it's important to remove history for inactive channels.
	HistoryTTL tools.Duration `mapstructure:"history_ttl" json:"history_ttl"`

	// Recover enables recovery mechanism for channels. This means that
	// server will try to recover missed messages for resubscribing client.
	// This option uses publications from history and must be used with reasonable
	// HistorySize and HistoryTTL configuration.
	Recover bool `mapstructure:"recover" json:"recover"`

	// Position enables client positioning.
	Position bool `mapstructure:"position" json:"position"`

	// Protected when on will prevent a client to subscribe to arbitrary channels in a
	// namespace. In this case Centrifugo will only allow client to subscribe on user-limited
	// channels, on channels returned by proxy response or channels listed inside JWT.
	// Client-side subscriptions to arbitrary channels will be rejected with PermissionDenied
	// error. Server-side channels belonging to protected namespace passed by client itself during
	// connect will be ignored.
	Protected bool `mapstructure:"protected" json:"protected"`

	// Publish enables possibility for clients to publish messages into channels.
	// Once enabled client can publish into channel and that publication will be
	// sent to all current channel subscribers.
	Publish bool `mapstructure:"publish" json:"publish"`

	// SubscribeToPublish turns on an automatic check that client subscribed
	// on a channel before allow publishing.
	SubscribeToPublish bool `mapstructure:"subscribe_to_publish" json:"subscribe_to_publish"`

	// Anonymous enables anonymous access (with empty user ID) to channel.
	// In most situations your application works with authenticated users so
	// every user has its own unique user ID. But if you provide real-time
	// features for public access you may need unauthenticated access to channels.
	// Turn on this option and use empty string as user ID.
	Anonymous bool `mapstructure:"anonymous" json:"anonymous"`

	// PresenceDisableForClient prevents presence to be asked by clients.
	// In this case it's available only over server-side presence call.
	PresenceDisableForClient bool `mapstructure:"presence_disable_for_client" json:"presence_disable_for_client"`

	// HistoryDisableForClient prevents history to be asked by clients.
	// In this case it's available only over server-side history call.
	// History recover mechanism if enabled will continue to work for
	// clients anyway.
	HistoryDisableForClient bool `mapstructure:"history_disable_for_client" json:"history_disable_for_client"`

	// ProxySubscribe turns on proxying subscribe decision for channels.
	ProxySubscribe bool `mapstructure:"proxy_subscribe" json:"proxy_subscribe"`

	// ProxyPublish turns on proxying publish decision for channels.
	ProxyPublish bool `mapstructure:"proxy_publish" json:"proxy_publish"`

	// SubscribeProxyName of proxy to use for subscribe operations in namespace.
	SubscribeProxyName string `mapstructure:"subscribe_proxy_name" json:"subscribe_proxy_name"`

	// PublishProxyName of proxy to use for publish operations in namespace.
	PublishProxyName string `mapstructure:"publish_proxy_name" json:"publish_proxy_name"`
}
