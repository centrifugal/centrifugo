package rule

import (
	"regexp"

	"github.com/centrifugal/centrifugo/v4/internal/tools"
)

// ChannelNamespace allows creating channels with different channel options.
type ChannelNamespace struct {
	// Name is a unique namespace name.
	Name string `mapstructure:"name" json:"name"`

	// Options for namespace determine channel options for channels
	// belonging to this namespace.
	ChannelOptions `mapstructure:",squash"`
}

type Compiled struct {
	CompiledChannelRegex *regexp.Regexp
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

	// ForcePushJoinLeave forces sending join/leave messages towards subscribers.
	ForcePushJoinLeave bool `mapstructure:"force_push_join_leave" json:"force_push_join_leave"`

	// HistorySize determines max amount of history messages for a channel,
	// Zero value means no history for channel. Centrifuge history has an
	// auxiliary role with current Engines – it can not replace your backend
	// persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size"`

	// HistoryTTL is a time to live for history cache. Server maintains a window of
	// messages in memory (or in Redis with Redis engine), to prevent infinite memory
	// grows it's important to remove history for inactive channels.
	HistoryTTL tools.Duration `mapstructure:"history_ttl" json:"history_ttl"`

	// ForcePositioning enables client positioning. This means that StreamPosition
	// will be exposed to the client and server will look that no messages from
	// PUB/SUB layer lost. In the loss found – client is disconnected (or unsubscribed)
	// with reconnect (resubscribe) code.
	ForcePositioning bool `mapstructure:"force_positioning" json:"force_positioning"`

	// AllowPositioning allows positioning when client asks about it.
	AllowPositioning bool `mapstructure:"allow_positioning" json:"allow_positioning"`

	// ForceRecovery enables recovery mechanism for channels. This means that
	// server will try to recover missed messages for resubscribing client.
	// This option uses publications from history and must be used with reasonable
	// HistorySize and HistoryTTL configuration.
	ForceRecovery bool `mapstructure:"force_recovery" json:"force_recovery"`

	// AllowRecovery allows recovery when client asks about it.
	AllowRecovery bool `mapstructure:"allow_recovery" json:"allow_recovery"`

	// SubscribeForAnonymous ...
	SubscribeForAnonymous bool `mapstructure:"allow_subscribe_for_anonymous" json:"allow_subscribe_for_anonymous"`

	// SubscribeForClient ...
	SubscribeForClient bool `mapstructure:"allow_subscribe_for_client" json:"allow_subscribe_for_client"`

	// PublishForAnonymous ...
	PublishForAnonymous bool `mapstructure:"allow_publish_for_anonymous" json:"allow_publish_for_anonymous"`

	// PublishForSubscriber ...
	PublishForSubscriber bool `mapstructure:"allow_publish_for_subscriber" json:"allow_publish_for_subscriber"`

	// PublishForClient ...
	PublishForClient bool `mapstructure:"allow_publish_for_client" json:"allow_publish_for_client"`

	// PresenceForAnonymous ...
	PresenceForAnonymous bool `mapstructure:"allow_presence_for_anonymous" json:"allow_presence_for_anonymous"`

	// PresenceForSubscriber ...
	PresenceForSubscriber bool `mapstructure:"allow_presence_for_subscriber" json:"allow_presence_for_subscriber"`

	// PresenceForClient ...
	PresenceForClient bool `mapstructure:"allow_presence_for_client" json:"allow_presence_for_client"`

	// HistoryForAnonymous ...
	HistoryForAnonymous bool `mapstructure:"allow_history_for_anonymous" json:"allow_history_for_anonymous"`

	// HistoryForSubscriber ...
	HistoryForSubscriber bool `mapstructure:"allow_history_for_subscriber" json:"allow_history_for_subscriber"`

	// HistoryForClient ...
	HistoryForClient bool `mapstructure:"allow_history_for_client" json:"allow_history_for_client"`

	// UserLimitedChannels ...
	UserLimitedChannels bool `mapstructure:"allow_user_limited_channels" json:"allow_user_limited_channels"`

	// ChannelRegex ...
	ChannelRegex string `mapstructure:"channel_regex" json:"channel_regex"`

	// ProxySubscribe turns on proxying subscribe decision for channels.
	ProxySubscribe bool `mapstructure:"proxy_subscribe" json:"proxy_subscribe"`

	// ProxyPublish turns on proxying publish decision for channels.
	ProxyPublish bool `mapstructure:"proxy_publish" json:"proxy_publish"`

	// ProxySubRefresh turns on proxying sub refresh for channels.
	ProxySubRefresh bool `mapstructure:"proxy_sub_refresh" json:"proxy_sub_refresh"`

	// SubscribeProxyName of proxy to use for subscribe operations in namespace.
	SubscribeProxyName string `mapstructure:"subscribe_proxy_name" json:"subscribe_proxy_name"`

	// PublishProxyName of proxy to use for publish operations in namespace.
	PublishProxyName string `mapstructure:"publish_proxy_name" json:"publish_proxy_name"`

	// SubRefreshProxyName of proxy to use for sub refresh operations in namespace.
	SubRefreshProxyName string `mapstructure:"sub_refresh_proxy_name" json:"sub_refresh_proxy_name"`

	Compiled
}
