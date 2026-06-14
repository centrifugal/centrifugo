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
	Name string `mapstructure:"name" json:"name" envconfig:"name" yaml:"name" toml:"name" expose:"full" doc:"Unique namespace name. Channels prefixed with <<name:>> use this namespace's options."`

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
	Presence bool `mapstructure:"presence" json:"presence" envconfig:"presence" yaml:"presence" toml:"presence" doc:"Enables presence information for channels in this namespace — the set of clients currently subscribed. Adds storage and CPU overhead, so avoid on channels with very many subscribers."`

	// JoinLeave turns on join/leave messages for a channel.
	// When client subscribes on a channel join message sent to all
	// subscribers in this channel (including current client). When client
	// leaves channel (unsubscribes) leave message sent. This option does
	// not fit well for channels with many subscribers because every
	// subscribe/unsubscribe event results into join/leave event broadcast
	// to all other active subscribers thus overloads server with tons of
	// messages. Use accurately for channels with small number of active
	// subscribers.
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave" envconfig:"join_leave" yaml:"join_leave" toml:"join_leave" doc:"Enables join/leave messages broadcast to subscribers when clients subscribe or unsubscribe. Each event is sent to all subscribers, so avoid on channels with many subscribers."`

	// ForcePushJoinLeave forces sending join/leave messages towards subscribers.
	ForcePushJoinLeave bool `mapstructure:"force_push_join_leave" json:"force_push_join_leave" envconfig:"force_push_join_leave" yaml:"force_push_join_leave" toml:"force_push_join_leave" doc:"Forces pushing join/leave messages to subscribers even if they did not request them on subscribe."`

	// MapClientsPresenceChannelPrefix is a prefix for map-based client presence channel.
	// When set, client presence is tracked via MapBroker in a channel formed by this prefix + original channel name.
	MapClientsPresenceChannelPrefix string `mapstructure:"map_clients_presence_channel_prefix" json:"map_clients_presence_channel_prefix" envconfig:"map_clients_presence_channel_prefix" yaml:"map_clients_presence_channel_prefix" toml:"map_clients_presence_channel_prefix" expose:"full" doc:"Prefix for the map-based client presence channel. When set, client presence is tracked via MapBroker in a channel formed as this prefix plus the original channel name."`

	// MapUsersPresenceChannelPrefix is a prefix for map-based user presence channel.
	// When set, user presence is tracked via MapBroker in a channel formed by this prefix + original channel name.
	MapUsersPresenceChannelPrefix string `mapstructure:"map_users_presence_channel_prefix" json:"map_users_presence_channel_prefix" envconfig:"map_users_presence_channel_prefix" yaml:"map_users_presence_channel_prefix" toml:"map_users_presence_channel_prefix" expose:"full" doc:"Prefix for the map-based user presence channel. When set, user presence is tracked via MapBroker in a channel formed as this prefix plus the original channel name."`

	// HistorySize determines max amount of history messages for a channel,
	// Zero value means no history for channel. Centrifuge history has an
	// auxiliary role with current Engines – it can not replace your backend
	// persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size" envconfig:"history_size" yaml:"history_size" toml:"history_size" doc:"Maximum number of messages kept in channel history. Zero (default) disables history. History is auxiliary and must not replace your persistent storage."`

	// HistoryTTL is a time to live for history cache. Server maintains a window of
	// messages in memory (or in Redis with Redis engine), to prevent infinite memory
	// grows it's important to remove history for inactive channels.
	HistoryTTL Duration `mapstructure:"history_ttl" json:"history_ttl" envconfig:"history_ttl" yaml:"history_ttl" toml:"history_ttl" doc:"How long messages are retained in channel history, e.g. <<300s>>. Keep it bounded so history for inactive channels is freed."`

	// HistoryMetaTTL is a time to live for history stream meta information. Must be
	// much larger than HistoryTTL in common scenario. If zero, then we use global value
	// set over default_history_meta_ttl on configuration top level.
	HistoryMetaTTL Duration `mapstructure:"history_meta_ttl" json:"history_meta_ttl" envconfig:"history_meta_ttl" yaml:"history_meta_ttl" toml:"history_meta_ttl" doc:"How long history stream meta information (offset, epoch) is retained. Should be much larger than history_ttl. If zero, the global default_history_meta_ttl is used."`

	// ForcePositioning enables client positioning. This means that StreamPosition
	// will be exposed to the client and server will look that no messages from
	// PUB/SUB layer lost. In the loss found – client is disconnected (or unsubscribed)
	// with reconnect (resubscribe) code.
	ForcePositioning bool `mapstructure:"force_positioning" json:"force_positioning" envconfig:"force_positioning" yaml:"force_positioning" toml:"force_positioning" doc:"Forces positioning for all subscribers — Centrifugo checks no messages are lost in the PUB/SUB layer and disconnects the client with reconnect on detected loss. Requires reasonable history_size and history_ttl."`

	// AllowPositioning allows positioning when client asks about it.
	AllowPositioning bool `mapstructure:"allow_positioning" json:"allow_positioning" envconfig:"allow_positioning" yaml:"allow_positioning" toml:"allow_positioning" doc:"Allows clients to enable positioning on their own when subscribing. Requires reasonable history_size and history_ttl."`

	// ForceRecovery enables recovery mechanism for channels. This means that
	// server will try to recover missed messages for resubscribing client.
	// This option uses publications from history and must be used with reasonable
	// HistorySize and HistoryTTL configuration.
	ForceRecovery bool `mapstructure:"force_recovery" json:"force_recovery" envconfig:"force_recovery" yaml:"force_recovery" toml:"force_recovery" doc:"Forces recovery of missed messages for resubscribing clients using channel history. Requires reasonable history_size and history_ttl."`

	// AllowRecovery allows recovery when client asks about it.
	AllowRecovery bool `mapstructure:"allow_recovery" json:"allow_recovery" envconfig:"allow_recovery" yaml:"allow_recovery" toml:"allow_recovery" doc:"Allows clients to enable recovery on their own when subscribing. Requires reasonable history_size and history_ttl."`

	// ForceRecoveryMode can set the recovery mode for all channel subscribers in the namespace which use recovery.
	ForceRecoveryMode string `mapstructure:"force_recovery_mode" json:"force_recovery_mode" envconfig:"force_recovery_mode" yaml:"force_recovery_mode" toml:"force_recovery_mode" expose:"full" doc:"Recovery mode for subscribers in this namespace that use recovery. Use <<stream>> (default) or <<cache>> (only the latest publication is recovered)."`

	// AutoCacheRecovery makes Centrifugo automatically recover all subscriptions in the namespace
	// on subscribe, without requiring the subscriber to request recovery itself. In cache recovery
	// mode this means the latest publication is delivered on every (re)subscribe without the need
	// to provide an empty "since" on the client side, and it also enables this delivery for
	// server-side subscriptions (e.g. of unidirectional clients which may not even know channel
	// names). Requires force_recovery and force_recovery_mode set to "cache".
	AutoCacheRecovery bool `mapstructure:"auto_cache_recovery" json:"auto_cache_recovery" envconfig:"auto_cache_recovery" yaml:"auto_cache_recovery" toml:"auto_cache_recovery" doc:"Automatically recovers all subscriptions in the namespace on subscribe without the subscriber requesting recovery. In cache recovery mode delivers the latest publication on every (re)subscribe (no client-side empty \"since\" needed) and also works for server-side subscriptions of unidirectional clients. Requires force_recovery and force_recovery_mode=cache."`

	// AllowedDeltaTypes is non-empty contains slice of allowed delta types for subscribers to use.
	AllowedDeltaTypes []centrifuge.DeltaType `mapstructure:"allowed_delta_types" json:"allowed_delta_types" envconfig:"allowed_delta_types" yaml:"allowed_delta_types" toml:"allowed_delta_types" doc:"Delta types subscribers may request in this namespace, e.g. <<[\"fossil\"]>>. Empty disables delta compression."`

	// AllowTagsFilter enables tags filtering for channels in namespace. Clients can pass tags filter in subscribe request.
	// When tags filter is set only messages with matching tags will be delivered to the client.
	AllowTagsFilter bool `mapstructure:"allow_tags_filter" json:"allow_tags_filter" envconfig:"allow_tags_filter" yaml:"allow_tags_filter" toml:"allow_tags_filter" doc:"Allows clients to pass a tags filter on subscribe so only publications with matching tags are delivered to them."`

	// DeltaPublish enables delta publish mechanism for all messages published in namespace channels
	// without explicit flag usage in publish API request. Setting this option does not guarantee that
	// publication will be compressed when going towards subscribers – it still depends on subscriber
	// connection options and whether Centrifugo Node is able to find previous publication in channel.
	DeltaPublish bool `mapstructure:"delta_publish" json:"delta_publish" envconfig:"delta_publish" yaml:"delta_publish" toml:"delta_publish" doc:"Enables delta publish for all messages in the namespace without setting the flag per publish call. Actual compression still depends on subscriber connection options."`

	// SubscribeForAnonymous allows anonymous clients to subscribe on channels in namespace.
	SubscribeForAnonymous bool `mapstructure:"allow_subscribe_for_anonymous" json:"allow_subscribe_for_anonymous" envconfig:"allow_subscribe_for_anonymous" yaml:"allow_subscribe_for_anonymous" toml:"allow_subscribe_for_anonymous" doc:"Allows anonymous (empty user ID) clients to subscribe to channels in this namespace."`

	// SubscribeForClient allows authenticated clients to subscribe on channels in namespace.
	SubscribeForClient bool `mapstructure:"allow_subscribe_for_client" json:"allow_subscribe_for_client" envconfig:"allow_subscribe_for_client" yaml:"allow_subscribe_for_client" toml:"allow_subscribe_for_client" doc:"Allows authenticated (non-anonymous) clients to subscribe to channels in this namespace."`

	// PublishForAnonymous allows anonymous clients to publish messages into channels in namespace.
	PublishForAnonymous bool `mapstructure:"allow_publish_for_anonymous" json:"allow_publish_for_anonymous" envconfig:"allow_publish_for_anonymous" yaml:"allow_publish_for_anonymous" toml:"allow_publish_for_anonymous" doc:"Allows anonymous clients to publish directly into channels in this namespace. Enabling client-side publish bypasses your backend — use with care."`

	// PublishForSubscriber allows clients subscribed on channel to publish messages into it.
	PublishForSubscriber bool `mapstructure:"allow_publish_for_subscriber" json:"allow_publish_for_subscriber" envconfig:"allow_publish_for_subscriber" yaml:"allow_publish_for_subscriber" toml:"allow_publish_for_subscriber" doc:"Allows clients to publish into channels they are subscribed to. Client-side publish bypasses your backend — use with care."`

	// PublishForClient allows authenticated clients to publish messages into channels in namespace.
	PublishForClient bool `mapstructure:"allow_publish_for_client" json:"allow_publish_for_client" envconfig:"allow_publish_for_client" yaml:"allow_publish_for_client" toml:"allow_publish_for_client" doc:"Allows authenticated clients to publish into channels in this namespace even without being subscribed. Client-side publish bypasses your backend — use with care."`

	// PresenceForAnonymous allows anonymous clients to get presence information for channels in namespace.
	PresenceForAnonymous bool `mapstructure:"allow_presence_for_anonymous" json:"allow_presence_for_anonymous" envconfig:"allow_presence_for_anonymous" yaml:"allow_presence_for_anonymous" toml:"allow_presence_for_anonymous" doc:"Allows anonymous clients to call presence on channels in this namespace. Requires presence to be enabled."`

	// PresenceForSubscriber allows clients subscribed on channel to get presence information for it.
	PresenceForSubscriber bool `mapstructure:"allow_presence_for_subscriber" json:"allow_presence_for_subscriber" envconfig:"allow_presence_for_subscriber" yaml:"allow_presence_for_subscriber" toml:"allow_presence_for_subscriber" doc:"Allows clients to call presence on channels they are subscribed to. Requires presence to be enabled."`

	// PresenceForClient allows authenticated clients to get presence information for channels in namespace.
	PresenceForClient bool `mapstructure:"allow_presence_for_client" json:"allow_presence_for_client" envconfig:"allow_presence_for_client" yaml:"allow_presence_for_client" toml:"allow_presence_for_client" doc:"Allows authenticated clients to call presence on channels in this namespace even without being subscribed. Requires presence to be enabled."`

	// HistoryForAnonymous allows anonymous clients to get history information for channels in namespace.
	HistoryForAnonymous bool `mapstructure:"allow_history_for_anonymous" json:"allow_history_for_anonymous" envconfig:"allow_history_for_anonymous" yaml:"allow_history_for_anonymous" toml:"allow_history_for_anonymous" doc:"Allows anonymous clients to call history on channels in this namespace. Requires history to be enabled."`

	// HistoryForSubscriber allows clients subscribed on channel to get history information for it.
	HistoryForSubscriber bool `mapstructure:"allow_history_for_subscriber" json:"allow_history_for_subscriber" envconfig:"allow_history_for_subscriber" yaml:"allow_history_for_subscriber" toml:"allow_history_for_subscriber" doc:"Allows clients to call history on channels they are subscribed to. Requires history to be enabled."`

	// HistoryForClient allows authenticated clients to get history information for channels in namespace.
	HistoryForClient bool `mapstructure:"allow_history_for_client" json:"allow_history_for_client" envconfig:"allow_history_for_client" yaml:"allow_history_for_client" toml:"allow_history_for_client" doc:"Allows authenticated clients to call history on channels in this namespace even without being subscribed. Requires history to be enabled."`

	// UserLimitedChannels allows to limit number of channels user can subscribe to in namespace.
	UserLimitedChannels bool `mapstructure:"allow_user_limited_channels" json:"allow_user_limited_channels" envconfig:"allow_user_limited_channels" yaml:"allow_user_limited_channels" toml:"allow_user_limited_channels" doc:"Allows user-limited channels in this namespace — channels containing a <<#>> with a comma-separated list of user IDs allowed to subscribe."`

	// ChannelRegex sets a regular expression to check channel name against.
	ChannelRegex string `mapstructure:"channel_regex" json:"channel_regex" envconfig:"channel_regex" yaml:"channel_regex" toml:"channel_regex" expose:"full" doc:"Regular expression that channel names in this namespace (the part after the namespace prefix) must match. Empty disables the check."`

	// PublicationDataFormat defines the format validation for publication data.
	// If empty (default) - current behavior is used, reject empty data.
	// If "json" - validate that data is valid JSON, return bad request if not.
	// If "json_object" - validate that data is valid JSON and specifically a JSON object, return bad request if not.
	// If "binary" - allow empty data to be published.
	PublicationDataFormat string `mapstructure:"publication_data_format" json:"publication_data_format" envconfig:"publication_data_format" yaml:"publication_data_format" toml:"publication_data_format" expose:"full" doc:"Validation applied to publication data. Empty (default) rejects empty data; <<json>> requires valid JSON; <<json_object>> requires a JSON object; <<binary>> allows empty/binary data."`

	// SubscribeProxyEnabled turns on using proxy for subscribe operations in namespace.
	SubscribeProxyEnabled bool `mapstructure:"subscribe_proxy_enabled" json:"subscribe_proxy_enabled" envconfig:"subscribe_proxy_enabled" yaml:"subscribe_proxy_enabled" toml:"subscribe_proxy_enabled" doc:"Proxies subscribe events in this namespace to your backend for authorization. Requires a configured subscribe proxy."`
	// SubscribeProxyName of proxy to use for subscribe operations in namespace.
	SubscribeProxyName string `mapstructure:"subscribe_proxy_name" default:"default" json:"subscribe_proxy_name" envconfig:"subscribe_proxy_name" yaml:"subscribe_proxy_name" toml:"subscribe_proxy_name" expose:"full" doc:"Name of the configured proxy to use for subscribe events in this namespace. Defaults to <<default>>."`

	// PublishProxyEnabled turns on using proxy for publish operations in namespace.
	PublishProxyEnabled bool `mapstructure:"publish_proxy_enabled" json:"publish_proxy_enabled" envconfig:"publish_proxy_enabled" yaml:"publish_proxy_enabled" toml:"publish_proxy_enabled" doc:"Proxies client publish events in this namespace to your backend for authorization or transformation. Requires a configured publish proxy."`
	// PublishProxyName of proxy to use for publish operations in namespace.
	PublishProxyName string `mapstructure:"publish_proxy_name" default:"default" json:"publish_proxy_name" envconfig:"publish_proxy_name" yaml:"publish_proxy_name" toml:"publish_proxy_name" expose:"full" doc:"Name of the configured proxy to use for publish events in this namespace. Defaults to <<default>>."`

	// SubRefreshProxyEnabled turns on using proxy for sub refresh operations in namespace.
	SubRefreshProxyEnabled bool `mapstructure:"sub_refresh_proxy_enabled" json:"sub_refresh_proxy_enabled" envconfig:"sub_refresh_proxy_enabled" yaml:"sub_refresh_proxy_enabled" toml:"sub_refresh_proxy_enabled" doc:"Proxies subscription refresh events in this namespace to your backend to validate and prolong subscriptions. Requires a configured sub refresh proxy."`
	// SubRefreshProxyName of proxy to use for sub refresh operations in namespace.
	SubRefreshProxyName string `mapstructure:"sub_refresh_proxy_name" default:"default" json:"sub_refresh_proxy_name" envconfig:"sub_refresh_proxy_name" yaml:"sub_refresh_proxy_name" toml:"sub_refresh_proxy_name" expose:"full" doc:"Name of the configured proxy to use for subscription refresh events in this namespace. Defaults to <<default>>."`

	// SubscribeStreamProxyEnabled turns on using proxy for subscribe stream operations in namespace.
	SubscribeStreamProxyEnabled bool `mapstructure:"subscribe_stream_proxy_enabled" json:"subscribe_stream_proxy_enabled" envconfig:"subscribe_stream_proxy_enabled" yaml:"subscribe_stream_proxy_enabled" toml:"subscribe_stream_proxy_enabled" doc:"Proxies subscriptions in this namespace to a stream proxy that streams publications from your backend instead of the PUB/SUB engine. Requires a configured subscribe stream proxy."`
	// SubscribeStreamProxyName of proxy to use for subscribe stream operations in namespace.
	SubscribeStreamProxyName string `mapstructure:"subscribe_stream_proxy_name" default:"default" json:"subscribe_stream_proxy_name" envconfig:"subscribe_stream_proxy_name" yaml:"subscribe_stream_proxy_name" toml:"subscribe_stream_proxy_name" expose:"full" doc:"Name of the configured proxy to use for subscribe stream in this namespace. Defaults to <<default>>."`
	// SubscribeStreamBidirectional enables using bidirectional stream proxy for the namespace.
	SubscribeStreamBidirectional bool `mapstructure:"subscribe_stream_proxy_bidirectional" json:"subscribe_stream_proxy_bidirectional" envconfig:"subscribe_stream_proxy_bidirectional" yaml:"subscribe_stream_proxy_bidirectional" toml:"subscribe_stream_proxy_bidirectional" doc:"Enables bidirectional mode for the subscribe stream proxy, letting clients also send data into the stream."`

	// SubscriptionType defines the subscription type for the namespace.
	// Valid values: "stream", "map", "map_clients", "map_users", "shared_poll".
	// Default: "stream" (traditional PUB/SUB with history).
	SubscriptionType string `mapstructure:"subscription_type" json:"subscription_type" envconfig:"subscription_type" yaml:"subscription_type" toml:"subscription_type" expose:"full" doc:"Subscription mechanism for the namespace. One of <<stream>> (default, PUB/SUB with history), <<map>>, <<map_clients>>, <<map_users>>, or <<shared_poll>>."`

	// Map contains configuration for map subscription types (map, map_clients, map_users).
	Map MapConfig `mapstructure:"map" json:"map" envconfig:"map" yaml:"map" toml:"map" doc:"Configuration for the map subscription types (<<map>>, <<map_clients>>, <<map_users>>)."`

	// SharedPoll contains configuration for shared poll subscription type.
	SharedPoll SharedPollConfig `mapstructure:"shared_poll" json:"shared_poll" envconfig:"shared_poll" yaml:"shared_poll" toml:"shared_poll" doc:"Configuration for the <<shared_poll>> subscription type."`

	Compiled `json:"-" yaml:"-" toml:"-"`
}

// MapConfig contains configuration for map subscription types (map, map_clients, map_users).
type MapConfig struct {
	Mode                              string   `mapstructure:"mode" json:"mode" envconfig:"mode" yaml:"mode" toml:"mode" expose:"full" doc:"Map mode controlling how keys are stored and delivered. See map subscription docs for supported values."`
	KeyTTL                            Duration `mapstructure:"key_ttl" json:"key_ttl" envconfig:"key_ttl" yaml:"key_ttl" toml:"key_ttl" doc:"How long a key is retained in the map after its last update, e.g. <<300s>>. Zero means no expiration."`
	Ordered                           bool     `mapstructure:"ordered" json:"ordered" envconfig:"ordered" yaml:"ordered" toml:"ordered" doc:"Preserves key ordering when delivering the map state to subscribers."`
	StreamSize                        int      `mapstructure:"stream_size" json:"stream_size" envconfig:"stream_size" yaml:"stream_size" toml:"stream_size" doc:"Maximum number of updates kept in the per-channel change stream used to bring subscribers up to date."`
	StreamTTL                         Duration `mapstructure:"stream_ttl" json:"stream_ttl" envconfig:"stream_ttl" yaml:"stream_ttl" toml:"stream_ttl" doc:"How long the per-channel change stream is retained, e.g. <<60s>>."`
	MetaTTL                           Duration `mapstructure:"meta_ttl" json:"meta_ttl" envconfig:"meta_ttl" yaml:"meta_ttl" toml:"meta_ttl" doc:"How long map stream meta information is retained. Should be larger than stream_ttl."`
	RemoveClientOnUnsubscribe         bool     `mapstructure:"remove_client_on_unsubscribe" json:"remove_client_on_unsubscribe" envconfig:"remove_client_on_unsubscribe" yaml:"remove_client_on_unsubscribe" toml:"remove_client_on_unsubscribe" doc:"Removes the client's own key from the map automatically when it unsubscribes."`
	ClientKey                         string   `mapstructure:"client_key" json:"client_key" envconfig:"client_key" yaml:"client_key" toml:"client_key" doc:"Template for the key a client writes into the map, e.g. based on client or user ID."`
	AllowPublishForClient             bool     `mapstructure:"allow_publish_for_client" json:"allow_publish_for_client" envconfig:"allow_publish_for_client" yaml:"allow_publish_for_client" toml:"allow_publish_for_client" doc:"Allows authenticated clients to write keys into the map. Client-side writes bypass your backend — use with care."`
	AllowPublishForSubscriber         bool     `mapstructure:"allow_publish_for_subscriber" json:"allow_publish_for_subscriber" envconfig:"allow_publish_for_subscriber" yaml:"allow_publish_for_subscriber" toml:"allow_publish_for_subscriber" doc:"Allows subscribed clients to write keys into the map. Client-side writes bypass your backend — use with care."`
	AllowPublishForAnonymous          bool     `mapstructure:"allow_publish_for_anonymous" json:"allow_publish_for_anonymous" envconfig:"allow_publish_for_anonymous" yaml:"allow_publish_for_anonymous" toml:"allow_publish_for_anonymous" doc:"Allows anonymous clients to write keys into the map. Client-side writes bypass your backend — use with care."`
	AllowRemoveForClient              bool     `mapstructure:"allow_remove_for_client" json:"allow_remove_for_client" envconfig:"allow_remove_for_client" yaml:"allow_remove_for_client" toml:"allow_remove_for_client" doc:"Allows authenticated clients to remove keys from the map. Use with care."`
	AllowRemoveForSubscriber          bool     `mapstructure:"allow_remove_for_subscriber" json:"allow_remove_for_subscriber" envconfig:"allow_remove_for_subscriber" yaml:"allow_remove_for_subscriber" toml:"allow_remove_for_subscriber" doc:"Allows subscribed clients to remove keys from the map. Use with care."`
	AllowRemoveForAnonymous           bool     `mapstructure:"allow_remove_for_anonymous" json:"allow_remove_for_anonymous" envconfig:"allow_remove_for_anonymous" yaml:"allow_remove_for_anonymous" toml:"allow_remove_for_anonymous" doc:"Allows anonymous clients to remove keys from the map. Use with care."`
	PublishProxyEnabled               bool     `mapstructure:"publish_proxy_enabled" json:"publish_proxy_enabled" envconfig:"publish_proxy_enabled" yaml:"publish_proxy_enabled" toml:"publish_proxy_enabled" doc:"Proxies map key writes to your backend for authorization or transformation. Requires a configured publish proxy."`
	PublishProxyName                  string   `mapstructure:"publish_proxy_name" default:"default" json:"publish_proxy_name" envconfig:"publish_proxy_name" yaml:"publish_proxy_name" toml:"publish_proxy_name" expose:"full" doc:"Name of the configured proxy to use for map key writes. Defaults to <<default>>."`
	RemoveProxyEnabled                bool     `mapstructure:"remove_proxy_enabled" json:"remove_proxy_enabled" envconfig:"remove_proxy_enabled" yaml:"remove_proxy_enabled" toml:"remove_proxy_enabled" doc:"Proxies map key removals to your backend for authorization. Requires a configured remove proxy."`
	RemoveProxyName                   string   `mapstructure:"remove_proxy_name" default:"default" json:"remove_proxy_name" envconfig:"remove_proxy_name" yaml:"remove_proxy_name" toml:"remove_proxy_name" expose:"full" doc:"Name of the configured proxy to use for map key removals. Defaults to <<default>>."`
	DefaultPageSize                   int      `mapstructure:"default_page_size" json:"default_page_size" envconfig:"default_page_size" yaml:"default_page_size" toml:"default_page_size" doc:"Default number of keys returned per page when a client does not specify a page size."`
	MinPageSize                       int      `mapstructure:"min_page_size" json:"min_page_size" envconfig:"min_page_size" yaml:"min_page_size" toml:"min_page_size" doc:"Minimum page size a client may request when paginating map keys."`
	MaxPageSize                       int      `mapstructure:"max_page_size" json:"max_page_size" envconfig:"max_page_size" yaml:"max_page_size" toml:"max_page_size" doc:"Maximum page size a client may request when paginating map keys."`
	LiveTransitionMaxPublicationLimit int      `mapstructure:"live_transition_max_publication_limit" json:"live_transition_max_publication_limit" envconfig:"live_transition_max_publication_limit" yaml:"live_transition_max_publication_limit" toml:"live_transition_max_publication_limit" doc:"Maximum number of buffered publications allowed when a subscriber transitions to the live state. Exceeding it falls back to a full state load."`
	SubscribeCatchUpTimeout           Duration `mapstructure:"subscribe_catch_up_timeout" json:"subscribe_catch_up_timeout" envconfig:"subscribe_catch_up_timeout" yaml:"subscribe_catch_up_timeout" toml:"subscribe_catch_up_timeout" doc:"Maximum time a subscriber may spend catching up to the live map state before the attempt is aborted, e.g. <<10s>>."`
}

// SharedPollConfig contains configuration for shared poll subscription type.
type SharedPollConfig struct {
	ProxyName              string   `mapstructure:"proxy_name" json:"proxy_name" envconfig:"proxy_name" yaml:"proxy_name" toml:"proxy_name" expose:"full" doc:"Name of the configured proxy used to poll state for shared-poll channels in this namespace."`
	RefreshInterval        Duration `mapstructure:"refresh_interval" json:"refresh_interval" envconfig:"refresh_interval" yaml:"refresh_interval" toml:"refresh_interval" doc:"How often Centrifugo polls the backend for fresh state, e.g. <<5s>>. Lower values reduce latency but increase backend load."`
	RefreshBatchSize       int      `mapstructure:"refresh_batch_size" json:"refresh_batch_size" envconfig:"refresh_batch_size" yaml:"refresh_batch_size" toml:"refresh_batch_size" doc:"Number of keys polled from the backend per refresh request."`
	MaxKeysPerConnection   int      `mapstructure:"max_keys_per_connection" json:"max_keys_per_connection" envconfig:"max_keys_per_connection" yaml:"max_keys_per_connection" toml:"max_keys_per_connection" doc:"Maximum number of shared-poll keys a single connection may subscribe to."`
	Mode                   string   `mapstructure:"mode" json:"mode" envconfig:"mode" yaml:"mode" toml:"mode" expose:"full" doc:"Shared poll mode controlling how state is fetched and delivered. See shared poll docs for supported values."`
	ChannelShutdownDelay   Duration `mapstructure:"channel_shutdown_delay" json:"channel_shutdown_delay" envconfig:"channel_shutdown_delay" yaml:"channel_shutdown_delay" toml:"channel_shutdown_delay" doc:"Delay before a channel with no subscribers stops polling and shuts down, e.g. <<10s>>. Avoids churn from quick resubscribes."`
	TrackExpiredExtraDelay Duration `mapstructure:"track_expired_extra_delay" json:"track_expired_extra_delay" envconfig:"track_expired_extra_delay" yaml:"track_expired_extra_delay" toml:"track_expired_extra_delay" doc:"Extra time an expired key keeps being tracked before removal, e.g. <<5s>>."`
	PublishEnabled         bool     `mapstructure:"publish_enabled" json:"publish_enabled" envconfig:"publish_enabled" yaml:"publish_enabled" toml:"publish_enabled" doc:"Allows publishing into shared-poll channels in addition to backend polling."`
}

type Compiled struct {
	CompiledChannelRegex *regexp.Regexp `json:"-" yaml:"-" toml:"-" envconfig:"-"`
}
