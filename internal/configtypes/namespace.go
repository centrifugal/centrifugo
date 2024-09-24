package configtypes

import (
	"regexp"
	"time"

	"github.com/centrifugal/centrifuge"
)

// ChannelNamespace allows creating channels with different channel options.
type ChannelNamespace struct {
	// Name is a unique namespace name.
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name"`

	// Options for namespace determine channel options for channels
	// belonging to this namespace.
	ChannelOptions `mapstructure:",squash" yaml:",inline"`
}

func (o ChannelOptions) GetRecoveryMode() centrifuge.RecoveryMode {
	if o.ForceRecoveryMode == "cache" {
		return centrifuge.RecoveryModeCache
	}
	return centrifuge.RecoveryModeStream
}

// ChannelOptions represent channel specific configuration for namespace
// or global channel options if set on top level of configuration.
type ChannelOptions struct {
	// Presence turns on presence information for channel. Presence has
	// information about all clients currently subscribed to a channel.
	Presence bool `mapstructure:"presence" json:"presence" envconfig:"presence" yaml:"presence" toml:"presence"`

	// JoinLeave turns on join/leave messages for a channel.
	// When client subscribes on a channel join message sent to all
	// subscribers in this channel (including current client). When client
	// leaves channel (unsubscribes) leave message sent. This option does
	// not fit well for channels with many subscribers because every
	// subscribe/unsubscribe event results into join/leave event broadcast
	// to all other active subscribers thus overloads server with tons of
	// messages. Use accurately for channels with small number of active
	// subscribers.
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave" envconfig:"join_leave" yaml:"join_leave" toml:"join_leave"`

	// ForcePushJoinLeave forces sending join/leave messages towards subscribers.
	ForcePushJoinLeave bool `mapstructure:"force_push_join_leave" json:"force_push_join_leave" envconfig:"force_push_join_leave" yaml:"force_push_join_leave" toml:"force_push_join_leave"`

	// HistorySize determines max amount of history messages for a channel,
	// Zero value means no history for channel. Centrifuge history has an
	// auxiliary role with current Engines – it can not replace your backend
	// persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size" envconfig:"history_size" yaml:"history_size" toml:"history_size"`

	// HistoryTTL is a time to live for history cache. Server maintains a window of
	// messages in memory (or in Redis with Redis engine), to prevent infinite memory
	// grows it's important to remove history for inactive channels.
	HistoryTTL time.Duration `mapstructure:"history_ttl" json:"history_ttl" envconfig:"history_ttl" yaml:"history_ttl" toml:"history_ttl"`

	// HistoryMetaTTL is a time to live for history stream meta information. Must be
	// much larger than HistoryTTL in common scenario. If zero, then we use global value
	// set over default_history_meta_ttl on configuration top level.
	HistoryMetaTTL time.Duration `mapstructure:"history_meta_ttl" json:"history_meta_ttl" envconfig:"history_meta_ttl" yaml:"history_meta_ttl" toml:"history_meta_ttl"`

	// ForcePositioning enables client positioning. This means that StreamPosition
	// will be exposed to the client and server will look that no messages from
	// PUB/SUB layer lost. In the loss found – client is disconnected (or unsubscribed)
	// with reconnect (resubscribe) code.
	ForcePositioning bool `mapstructure:"force_positioning" json:"force_positioning" envconfig:"force_positioning" yaml:"force_positioning" toml:"force_positioning"`

	// AllowPositioning allows positioning when client asks about it.
	AllowPositioning bool `mapstructure:"allow_positioning" json:"allow_positioning" envconfig:"allow_positioning" yaml:"allow_positioning" toml:"allow_positioning"`

	// ForceRecovery enables recovery mechanism for channels. This means that
	// server will try to recover missed messages for resubscribing client.
	// This option uses publications from history and must be used with reasonable
	// HistorySize and HistoryTTL configuration.
	ForceRecovery bool `mapstructure:"force_recovery" json:"force_recovery" envconfig:"force_recovery" yaml:"force_recovery" toml:"force_recovery"`

	// AllowRecovery allows recovery when client asks about it.
	AllowRecovery bool `mapstructure:"allow_recovery" json:"allow_recovery" envconfig:"allow_recovery" yaml:"allow_recovery" toml:"allow_recovery"`

	// ForceRecoveryMode can set the recovery mode for all channel subscribers in the namespace which use recovery.
	ForceRecoveryMode string `mapstructure:"force_recovery_mode" json:"force_recovery_mode" envconfig:"force_recovery_mode" yaml:"force_recovery_mode" toml:"force_recovery_mode"`

	// AllowedDeltaTypes is non-empty contains slice of allowed delta types for subscribers to use.
	AllowedDeltaTypes []centrifuge.DeltaType `mapstructure:"allowed_delta_types" json:"allowed_delta_types" envconfig:"allowed_delta_types" yaml:"allowed_delta_types" toml:"allowed_delta_types"`

	// DeltaPublish enables delta publish mechanism for all messages published in namespace channels
	// without explicit flag usage in publish API request. Setting this option does not guarantee that
	// publication will be compressed when going towards subscribers – it still depends on subscriber
	// connection options and whether Centrifugo Node is able to find previous publication in channel.
	DeltaPublish bool `mapstructure:"delta_publish" json:"delta_publish" envconfig:"delta_publish" yaml:"delta_publish" toml:"delta_publish"`

	// SubscribeForAnonymous ...
	SubscribeForAnonymous bool `mapstructure:"allow_subscribe_for_anonymous" json:"allow_subscribe_for_anonymous" envconfig:"allow_subscribe_for_anonymous" yaml:"allow_subscribe_for_anonymous" toml:"allow_subscribe_for_anonymous"`

	// SubscribeForClient ...
	SubscribeForClient bool `mapstructure:"allow_subscribe_for_client" json:"allow_subscribe_for_client" envconfig:"allow_subscribe_for_client" yaml:"allow_subscribe_for_client" toml:"allow_subscribe_for_client"`

	// PublishForAnonymous ...
	PublishForAnonymous bool `mapstructure:"allow_publish_for_anonymous" json:"allow_publish_for_anonymous" envconfig:"allow_publish_for_anonymous" yaml:"allow_publish_for_anonymous" toml:"allow_publish_for_anonymous"`

	// PublishForSubscriber ...
	PublishForSubscriber bool `mapstructure:"allow_publish_for_subscriber" json:"allow_publish_for_subscriber" envconfig:"allow_publish_for_subscriber" yaml:"allow_publish_for_subscriber" toml:"allow_publish_for_subscriber"`

	// PublishForClient ...
	PublishForClient bool `mapstructure:"allow_publish_for_client" json:"allow_publish_for_client" envconfig:"allow_publish_for_client" yaml:"allow_publish_for_client" toml:"allow_publish_for_client"`

	// PresenceForAnonymous ...
	PresenceForAnonymous bool `mapstructure:"allow_presence_for_anonymous" json:"allow_presence_for_anonymous" envconfig:"allow_presence_for_anonymous" yaml:"allow_presence_for_anonymous" toml:"allow_presence_for_anonymous"`

	// PresenceForSubscriber ...
	PresenceForSubscriber bool `mapstructure:"allow_presence_for_subscriber" json:"allow_presence_for_subscriber" envconfig:"allow_presence_for_subscriber" yaml:"allow_presence_for_subscriber" toml:"allow_presence_for_subscriber"`

	// PresenceForClient ...
	PresenceForClient bool `mapstructure:"allow_presence_for_client" json:"allow_presence_for_client" envconfig:"allow_presence_for_client" yaml:"allow_presence_for_client" toml:"allow_presence_for_client"`

	// HistoryForAnonymous ...
	HistoryForAnonymous bool `mapstructure:"allow_history_for_anonymous" json:"allow_history_for_anonymous" envconfig:"allow_history_for_anonymous" yaml:"allow_history_for_anonymous" toml:"allow_history_for_anonymous"`

	// HistoryForSubscriber ...
	HistoryForSubscriber bool `mapstructure:"allow_history_for_subscriber" json:"allow_history_for_subscriber" envconfig:"allow_history_for_subscriber" yaml:"allow_history_for_subscriber" toml:"allow_history_for_subscriber"`

	// HistoryForClient ...
	HistoryForClient bool `mapstructure:"allow_history_for_client" json:"allow_history_for_client" envconfig:"allow_history_for_client" yaml:"allow_history_for_client" toml:"allow_history_for_client"`

	// UserLimitedChannels ...
	UserLimitedChannels bool `mapstructure:"allow_user_limited_channels" json:"allow_user_limited_channels" envconfig:"allow_user_limited_channels" yaml:"allow_user_limited_channels" toml:"allow_user_limited_channels"`

	// ChannelRegex ...
	ChannelRegex string `mapstructure:"channel_regex" json:"channel_regex" envconfig:"channel_regex" yaml:"channel_regex" toml:"channel_regex"`

	// SubscribeProxyName of proxy to use for subscribe operations in namespace.
	SubscribeProxyName string `mapstructure:"subscribe_proxy_name" json:"subscribe_proxy_name" envconfig:"subscribe_proxy_name" yaml:"subscribe_proxy_name" toml:"subscribe_proxy_name"`

	// PublishProxyName of proxy to use for publish operations in namespace.
	PublishProxyName string `mapstructure:"publish_proxy_name" json:"publish_proxy_name" envconfig:"publish_proxy_name" yaml:"publish_proxy_name" toml:"publish_proxy_name"`

	// SubRefreshProxyName of proxy to use for sub refresh operations in namespace.
	SubRefreshProxyName string `mapstructure:"sub_refresh_proxy_name" json:"sub_refresh_proxy_name" envconfig:"sub_refresh_proxy_name" yaml:"sub_refresh_proxy_name" toml:"sub_refresh_proxy_name"`

	// CacheEmptyProxyName of proxy to use for cache empty operations in namespace.
	CacheEmptyProxyName string `mapstructure:"cache_empty_proxy_name" json:"cache_empty_proxy_name" envconfig:"cache_empty_proxy_name" yaml:"cache_empty_proxy_name" toml:"cache_empty_proxy_name"`

	// SubscribeStreamProxyName of proxy to use for subscribe stream operations in namespace.
	SubscribeStreamProxyName string `mapstructure:"subscribe_stream_proxy_name" json:"subscribe_stream_proxy_name" envconfig:"subscribe_stream_proxy_name" yaml:"subscribe_stream_proxy_name" toml:"subscribe_stream_proxy_name"`

	// SubscribeStreamBidirectional enables using bidirectional stream proxy for the namespace.
	SubscribeStreamBidirectional bool `mapstructure:"subscribe_stream_bidirectional" json:"subscribe_stream_bidirectional" envconfig:"subscribe_stream_bidirectional" yaml:"subscribe_stream_bidirectional" toml:"subscribe_stream_bidirectional"`

	Compiled `json:"-" yaml:"-" toml:"-"`
}

type Compiled struct {
	CompiledChannelRegex *regexp.Regexp `json:"-" yaml:"-" toml:"-"`
}
