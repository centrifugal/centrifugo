package configtypes

import (
	"regexp"
	"strings"

	"github.com/centrifugal/centrifuge"
)

const (
	// PublicationDataFormatJSON indicates that publication data must be valid JSON.
	PublicationDataFormatJSON = "json"
	// PublicationDataFormatJSONObject indicates that publication data must be valid JSON and specifically a JSON object.
	PublicationDataFormatJSONObject = "json_object"
	// PublicationDataFormatBinary indicates that publication data is binary and empty data is allowed.
	PublicationDataFormatBinary = "binary"
)

type ChannelNamespaces []ChannelNamespace

// Decode to implement the envconfig.Decoder interface
func (d *ChannelNamespaces) Decode(value string) error {
	return decodeToNamedSlice(value, d)
}

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

func NameForEnv(input string) string {
	// Create a new replacer to replace '.' and '-' with '_'
	replacer := strings.NewReplacer(".", "_", "-", "_")
	return replacer.Replace(input)
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

	// MapClientsPresenceChannelPrefix is a prefix for map-based client presence channel.
	// When set, client presence is tracked via MapBroker in a channel formed by this prefix + original channel name.
	MapClientsPresenceChannelPrefix string `mapstructure:"map_clients_presence_channel_prefix" json:"map_clients_presence_channel_prefix" envconfig:"map_clients_presence_channel_prefix" yaml:"map_clients_presence_channel_prefix" toml:"map_clients_presence_channel_prefix"`

	// MapUsersPresenceChannelPrefix is a prefix for map-based user presence channel.
	// When set, user presence is tracked via MapBroker in a channel formed by this prefix + original channel name.
	MapUsersPresenceChannelPrefix string `mapstructure:"map_users_presence_channel_prefix" json:"map_users_presence_channel_prefix" envconfig:"map_users_presence_channel_prefix" yaml:"map_users_presence_channel_prefix" toml:"map_users_presence_channel_prefix"`

	// HistorySize determines max amount of history messages for a channel,
	// Zero value means no history for channel. Centrifuge history has an
	// auxiliary role with current Engines – it can not replace your backend
	// persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size" envconfig:"history_size" yaml:"history_size" toml:"history_size"`

	// HistoryTTL is a time to live for history cache. Server maintains a window of
	// messages in memory (or in Redis with Redis engine), to prevent infinite memory
	// grows it's important to remove history for inactive channels.
	HistoryTTL Duration `mapstructure:"history_ttl" json:"history_ttl" envconfig:"history_ttl" yaml:"history_ttl" toml:"history_ttl"`

	// HistoryMetaTTL is a time to live for history stream meta information. Must be
	// much larger than HistoryTTL in common scenario. If zero, then we use global value
	// set over default_history_meta_ttl on configuration top level.
	HistoryMetaTTL Duration `mapstructure:"history_meta_ttl" json:"history_meta_ttl" envconfig:"history_meta_ttl" yaml:"history_meta_ttl" toml:"history_meta_ttl"`

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

	// AllowTagsFilter enables tags filtering for channels in namespace. Clients can pass tags filter in subscribe request.
	// When tags filter is set only messages with matching tags will be delivered to the client.
	AllowTagsFilter bool `mapstructure:"allow_tags_filter" json:"allow_tags_filter" envconfig:"allow_tags_filter" yaml:"allow_tags_filter" toml:"allow_tags_filter"`

	// DeltaPublish enables delta publish mechanism for all messages published in namespace channels
	// without explicit flag usage in publish API request. Setting this option does not guarantee that
	// publication will be compressed when going towards subscribers – it still depends on subscriber
	// connection options and whether Centrifugo Node is able to find previous publication in channel.
	DeltaPublish bool `mapstructure:"delta_publish" json:"delta_publish" envconfig:"delta_publish" yaml:"delta_publish" toml:"delta_publish"`

	// SubscribeForAnonymous allows anonymous clients to subscribe on channels in namespace.
	SubscribeForAnonymous bool `mapstructure:"allow_subscribe_for_anonymous" json:"allow_subscribe_for_anonymous" envconfig:"allow_subscribe_for_anonymous" yaml:"allow_subscribe_for_anonymous" toml:"allow_subscribe_for_anonymous"`

	// SubscribeForClient allows authenticated clients to subscribe on channels in namespace.
	SubscribeForClient bool `mapstructure:"allow_subscribe_for_client" json:"allow_subscribe_for_client" envconfig:"allow_subscribe_for_client" yaml:"allow_subscribe_for_client" toml:"allow_subscribe_for_client"`

	// PublishForAnonymous allows anonymous clients to publish messages into channels in namespace.
	PublishForAnonymous bool `mapstructure:"allow_publish_for_anonymous" json:"allow_publish_for_anonymous" envconfig:"allow_publish_for_anonymous" yaml:"allow_publish_for_anonymous" toml:"allow_publish_for_anonymous"`

	// PublishForSubscriber allows clients subscribed on channel to publish messages into it.
	PublishForSubscriber bool `mapstructure:"allow_publish_for_subscriber" json:"allow_publish_for_subscriber" envconfig:"allow_publish_for_subscriber" yaml:"allow_publish_for_subscriber" toml:"allow_publish_for_subscriber"`

	// PublishForClient allows authenticated clients to publish messages into channels in namespace.
	PublishForClient bool `mapstructure:"allow_publish_for_client" json:"allow_publish_for_client" envconfig:"allow_publish_for_client" yaml:"allow_publish_for_client" toml:"allow_publish_for_client"`

	// PresenceForAnonymous allows anonymous clients to get presence information for channels in namespace.
	PresenceForAnonymous bool `mapstructure:"allow_presence_for_anonymous" json:"allow_presence_for_anonymous" envconfig:"allow_presence_for_anonymous" yaml:"allow_presence_for_anonymous" toml:"allow_presence_for_anonymous"`

	// PresenceForSubscriber allows clients subscribed on channel to get presence information for it.
	PresenceForSubscriber bool `mapstructure:"allow_presence_for_subscriber" json:"allow_presence_for_subscriber" envconfig:"allow_presence_for_subscriber" yaml:"allow_presence_for_subscriber" toml:"allow_presence_for_subscriber"`

	// PresenceForClient allows authenticated clients to get presence information for channels in namespace.
	PresenceForClient bool `mapstructure:"allow_presence_for_client" json:"allow_presence_for_client" envconfig:"allow_presence_for_client" yaml:"allow_presence_for_client" toml:"allow_presence_for_client"`

	// HistoryForAnonymous allows anonymous clients to get history information for channels in namespace.
	HistoryForAnonymous bool `mapstructure:"allow_history_for_anonymous" json:"allow_history_for_anonymous" envconfig:"allow_history_for_anonymous" yaml:"allow_history_for_anonymous" toml:"allow_history_for_anonymous"`

	// HistoryForSubscriber allows clients subscribed on channel to get history information for it.
	HistoryForSubscriber bool `mapstructure:"allow_history_for_subscriber" json:"allow_history_for_subscriber" envconfig:"allow_history_for_subscriber" yaml:"allow_history_for_subscriber" toml:"allow_history_for_subscriber"`

	// HistoryForClient allows authenticated clients to get history information for channels in namespace.
	HistoryForClient bool `mapstructure:"allow_history_for_client" json:"allow_history_for_client" envconfig:"allow_history_for_client" yaml:"allow_history_for_client" toml:"allow_history_for_client"`

	// UserLimitedChannels allows to limit number of channels user can subscribe to in namespace.
	UserLimitedChannels bool `mapstructure:"allow_user_limited_channels" json:"allow_user_limited_channels" envconfig:"allow_user_limited_channels" yaml:"allow_user_limited_channels" toml:"allow_user_limited_channels"`

	// ChannelRegex sets a regular expression to check channel name against.
	ChannelRegex string `mapstructure:"channel_regex" json:"channel_regex" envconfig:"channel_regex" yaml:"channel_regex" toml:"channel_regex"`

	// PublicationDataFormat defines the format validation for publication data.
	// If empty (default) - current behavior is used, reject empty data.
	// If "json" - validate that data is valid JSON, return bad request if not.
	// If "json_object" - validate that data is valid JSON and specifically a JSON object, return bad request if not.
	// If "binary" - allow empty data to be published.
	PublicationDataFormat string `mapstructure:"publication_data_format" json:"publication_data_format" envconfig:"publication_data_format" yaml:"publication_data_format" toml:"publication_data_format"`

	// SubscribeProxyEnabled turns on using proxy for subscribe operations in namespace.
	SubscribeProxyEnabled bool `mapstructure:"subscribe_proxy_enabled" json:"subscribe_proxy_enabled" envconfig:"subscribe_proxy_enabled" yaml:"subscribe_proxy_enabled" toml:"subscribe_proxy_enabled"`
	// SubscribeProxyName of proxy to use for subscribe operations in namespace.
	SubscribeProxyName string `mapstructure:"subscribe_proxy_name" default:"default" json:"subscribe_proxy_name" envconfig:"subscribe_proxy_name" yaml:"subscribe_proxy_name" toml:"subscribe_proxy_name"`

	// PublishProxyEnabled turns on using proxy for publish operations in namespace.
	PublishProxyEnabled bool `mapstructure:"publish_proxy_enabled" json:"publish_proxy_enabled" envconfig:"publish_proxy_enabled" yaml:"publish_proxy_enabled" toml:"publish_proxy_enabled"`
	// PublishProxyName of proxy to use for publish operations in namespace.
	PublishProxyName string `mapstructure:"publish_proxy_name" default:"default" json:"publish_proxy_name" envconfig:"publish_proxy_name" yaml:"publish_proxy_name" toml:"publish_proxy_name"`

	// SubRefreshProxyEnabled turns on using proxy for sub refresh operations in namespace.
	SubRefreshProxyEnabled bool `mapstructure:"sub_refresh_proxy_enabled" json:"sub_refresh_proxy_enabled" envconfig:"sub_refresh_proxy_enabled" yaml:"sub_refresh_proxy_enabled" toml:"sub_refresh_proxy_enabled"`
	// SubRefreshProxyName of proxy to use for sub refresh operations in namespace.
	SubRefreshProxyName string `mapstructure:"sub_refresh_proxy_name" default:"default" json:"sub_refresh_proxy_name" envconfig:"sub_refresh_proxy_name" yaml:"sub_refresh_proxy_name" toml:"sub_refresh_proxy_name"`

	// SubscribeStreamProxyEnabled turns on using proxy for subscribe stream operations in namespace.
	SubscribeStreamProxyEnabled bool `mapstructure:"subscribe_stream_proxy_enabled" json:"subscribe_stream_proxy_enabled" envconfig:"subscribe_stream_proxy_enabled" yaml:"subscribe_stream_proxy_enabled" toml:"subscribe_stream_proxy_enabled"`
	// SubscribeStreamProxyName of proxy to use for subscribe stream operations in namespace.
	SubscribeStreamProxyName string `mapstructure:"subscribe_stream_proxy_name" default:"default" json:"subscribe_stream_proxy_name" envconfig:"subscribe_stream_proxy_name" yaml:"subscribe_stream_proxy_name" toml:"subscribe_stream_proxy_name"`
	// SubscribeStreamBidirectional enables using bidirectional stream proxy for the namespace.
	SubscribeStreamBidirectional bool `mapstructure:"subscribe_stream_proxy_bidirectional" json:"subscribe_stream_proxy_bidirectional" envconfig:"subscribe_stream_proxy_bidirectional" yaml:"subscribe_stream_proxy_bidirectional" toml:"subscribe_stream_proxy_bidirectional"`

	// SubscriptionType defines the subscription type for the namespace.
	// Valid values: "stream", "map", "map_clients", "map_users", "shared_poll".
	// Default: "stream" (traditional PUB/SUB with history).
	SubscriptionType string `mapstructure:"subscription_type" json:"subscription_type" envconfig:"subscription_type" yaml:"subscription_type" toml:"subscription_type"`

	// Map contains configuration for map subscription types (map, map_clients, map_users).
	Map MapConfig `mapstructure:"map" json:"map" envconfig:"map" yaml:"map" toml:"map"`

	// SharedPoll contains configuration for shared poll subscription type.
	SharedPoll SharedPollConfig `mapstructure:"shared_poll" json:"shared_poll" envconfig:"shared_poll" yaml:"shared_poll" toml:"shared_poll"`

	Compiled `json:"-" yaml:"-" toml:"-"`
}

// MapConfig contains configuration for map subscription types (map, map_clients, map_users).
type MapConfig struct {
	SyncMode                    string   `mapstructure:"sync_mode" json:"sync_mode" envconfig:"sync_mode" yaml:"sync_mode" toml:"sync_mode"`
	RetentionMode               string   `mapstructure:"retention_mode" json:"retention_mode" envconfig:"retention_mode" yaml:"retention_mode" toml:"retention_mode"`
	KeyTTL                      Duration `mapstructure:"key_ttl" json:"key_ttl" envconfig:"key_ttl" yaml:"key_ttl" toml:"key_ttl"`
	Ordered                     bool     `mapstructure:"ordered" json:"ordered" envconfig:"ordered" yaml:"ordered" toml:"ordered"`
	StreamSize                  int      `mapstructure:"stream_size" json:"stream_size" envconfig:"stream_size" yaml:"stream_size" toml:"stream_size"`
	StreamTTL                   Duration `mapstructure:"stream_ttl" json:"stream_ttl" envconfig:"stream_ttl" yaml:"stream_ttl" toml:"stream_ttl"`
	MetaTTL                     Duration `mapstructure:"meta_ttl" json:"meta_ttl" envconfig:"meta_ttl" yaml:"meta_ttl" toml:"meta_ttl"`
	RemoveClientOnUnsubscribe   bool     `mapstructure:"remove_client_on_unsubscribe" json:"remove_client_on_unsubscribe" envconfig:"remove_client_on_unsubscribe" yaml:"remove_client_on_unsubscribe" toml:"remove_client_on_unsubscribe"`
	ClientKey                   string   `mapstructure:"client_key" json:"client_key" envconfig:"client_key" yaml:"client_key" toml:"client_key"`
	AllowPublishForClient       bool     `mapstructure:"allow_publish_for_client" json:"allow_publish_for_client" envconfig:"allow_publish_for_client" yaml:"allow_publish_for_client" toml:"allow_publish_for_client"`
	AllowPublishForSubscriber   bool     `mapstructure:"allow_publish_for_subscriber" json:"allow_publish_for_subscriber" envconfig:"allow_publish_for_subscriber" yaml:"allow_publish_for_subscriber" toml:"allow_publish_for_subscriber"`
	AllowPublishForAnonymous    bool     `mapstructure:"allow_publish_for_anonymous" json:"allow_publish_for_anonymous" envconfig:"allow_publish_for_anonymous" yaml:"allow_publish_for_anonymous" toml:"allow_publish_for_anonymous"`
	AllowRemoveForClient        bool     `mapstructure:"allow_remove_for_client" json:"allow_remove_for_client" envconfig:"allow_remove_for_client" yaml:"allow_remove_for_client" toml:"allow_remove_for_client"`
	AllowRemoveForSubscriber    bool     `mapstructure:"allow_remove_for_subscriber" json:"allow_remove_for_subscriber" envconfig:"allow_remove_for_subscriber" yaml:"allow_remove_for_subscriber" toml:"allow_remove_for_subscriber"`
	AllowRemoveForAnonymous     bool     `mapstructure:"allow_remove_for_anonymous" json:"allow_remove_for_anonymous" envconfig:"allow_remove_for_anonymous" yaml:"allow_remove_for_anonymous" toml:"allow_remove_for_anonymous"`
	PublishProxyEnabled         bool     `mapstructure:"publish_proxy_enabled" json:"publish_proxy_enabled" envconfig:"publish_proxy_enabled" yaml:"publish_proxy_enabled" toml:"publish_proxy_enabled"`
	PublishProxyName            string   `mapstructure:"publish_proxy_name" default:"default" json:"publish_proxy_name" envconfig:"publish_proxy_name" yaml:"publish_proxy_name" toml:"publish_proxy_name"`
	RemoveProxyEnabled          bool     `mapstructure:"remove_proxy_enabled" json:"remove_proxy_enabled" envconfig:"remove_proxy_enabled" yaml:"remove_proxy_enabled" toml:"remove_proxy_enabled"`
	RemoveProxyName             string   `mapstructure:"remove_proxy_name" default:"default" json:"remove_proxy_name" envconfig:"remove_proxy_name" yaml:"remove_proxy_name" toml:"remove_proxy_name"`
}

// SharedPollConfig contains configuration for shared poll subscription type.
type SharedPollConfig struct {
	ProxyName              string   `mapstructure:"proxy_name" json:"proxy_name" envconfig:"proxy_name" yaml:"proxy_name" toml:"proxy_name"`
	RefreshInterval        Duration `mapstructure:"refresh_interval" json:"refresh_interval" envconfig:"refresh_interval" yaml:"refresh_interval" toml:"refresh_interval"`
	RefreshBatchSize       int      `mapstructure:"refresh_batch_size" json:"refresh_batch_size" envconfig:"refresh_batch_size" yaml:"refresh_batch_size" toml:"refresh_batch_size"`
	MaxKeysPerConnection   int      `mapstructure:"max_keys_per_connection" json:"max_keys_per_connection" envconfig:"max_keys_per_connection" yaml:"max_keys_per_connection" toml:"max_keys_per_connection"`
	RefreshMode            string   `mapstructure:"refresh_mode" json:"refresh_mode" envconfig:"refresh_mode" yaml:"refresh_mode" toml:"refresh_mode"`
	MaxConsecutiveAbsences int      `mapstructure:"max_consecutive_absences" json:"max_consecutive_absences" envconfig:"max_consecutive_absences" yaml:"max_consecutive_absences" toml:"max_consecutive_absences"`
	ChannelShutdownDelay   Duration `mapstructure:"channel_shutdown_delay" json:"channel_shutdown_delay" envconfig:"channel_shutdown_delay" yaml:"channel_shutdown_delay" toml:"channel_shutdown_delay"`
	TrackExpiredExtraDelay Duration `mapstructure:"track_expired_extra_delay" json:"track_expired_extra_delay" envconfig:"track_expired_extra_delay" yaml:"track_expired_extra_delay" toml:"track_expired_extra_delay"`
	PublishEnabled         bool     `mapstructure:"publish_enabled" json:"publish_enabled" envconfig:"publish_enabled" yaml:"publish_enabled" toml:"publish_enabled"`
}

type Compiled struct {
	CompiledChannelRegex *regexp.Regexp `json:"-" yaml:"-" toml:"-" envconfig:"-"`
}
