package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
	"github.com/centrifugal/centrifugo/v6/internal/tools"

	"github.com/centrifugal/centrifuge"
)

var knownBrokers = []string{"memory", "nats", "redis", "redisnats", "postgres"}

// Validate validates config and returns error if problems found.
func (c Config) Validate() error {
	if c.HTTP.H2CExternal && (c.HTTP.InternalPort == "" || strconv.Itoa(c.HTTP.Port) == c.HTTP.InternalPort) {
		return fmt.Errorf("external_h2c requires custom separate internal_port to be configured")
	}
	if c.HTTP.TLS.Enabled && c.HTTP.H2CExternal {
		return fmt.Errorf("external_h2c cannot be used together with enabled TLS for external HTTP server")
	}

	if !slices.Contains(knownBrokers, c.Broker.Type) {
		return fmt.Errorf("unknown broker: %s", c.Broker.Type)
	}

	if err := validateTokens(c); err != nil {
		return err
	}

	if c.Client.Proxy.Connect.Enabled {
		if err := validateProxy("default", c.Client.Proxy.Connect.Proxy); err != nil {
			return fmt.Errorf("in client.proxy.connect: %v", err)
		}
	}
	if c.Client.Proxy.Refresh.Enabled {
		if err := validateProxy("default", c.Client.Proxy.Refresh.Proxy); err != nil {
			return fmt.Errorf("in client.proxy.refresh: %v", err)
		}
	}

	if err := validateSecondPrecisionDuration(c.Channel.HistoryMetaTTL); err != nil {
		return fmt.Errorf("in channel.history_meta_ttl: %v", err)
	}

	if !slices.Contains([]string{"", configtypes.PublicationDataFormatJSON, configtypes.PublicationDataFormatJSONObject, configtypes.PublicationDataFormatBinary}, c.Channel.PublicationDataFormat) {
		return fmt.Errorf("unknown channel.publication_data_format: \"%s\"", c.Channel.PublicationDataFormat)
	}

	if err := validateCodeToUniDisconnectTransforms(c.Client.ConnectCodeToUnidirectionalDisconnect.Transforms); err != nil {
		return fmt.Errorf("in client.connect_code_to_unidirectional_disconnect: %v", err)
	}

	var proxyNames []string
	for _, p := range c.Proxies {
		if slices.Contains(proxyNames, p.Name) {
			return fmt.Errorf("duplicate proxy name in channel.named_proxies: %s", p.Name)
		}
		if err := validateProxy(p.Name, p.Proxy); err != nil {
			return fmt.Errorf("in proxy %s: %v", p.Name, err)
		}
		proxyNames = append(proxyNames, p.Name)
	}
	if slices.Contains(proxyNames, DefaultProxyName) {
		return fmt.Errorf("proxy name %s is reserved", DefaultProxyName)
	}
	proxyNames = append(proxyNames, DefaultProxyName) // channel options can use default proxy name.

	if err := validateChannelOptions(c.Channel.WithoutNamespace, c.Channel.HistoryMetaTTL, proxyNames, c); err != nil {
		return fmt.Errorf("in channel.without_namespace: %v", err)
	}

	if err := validateRpcOptions(c.RPC.WithoutNamespace, proxyNames); err != nil {
		return fmt.Errorf("in rpc.without_namespace: %v", err)
	}

	usePersonalChannel := c.Client.SubscribeToUserPersonalChannel.Enabled
	personalChannelNamespace := c.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace
	personalSingleConnection := c.Client.SubscribeToUserPersonalChannel.SingleConnection
	var validPersonalChannelNamespace bool
	if !usePersonalChannel || personalChannelNamespace == "" {
		validPersonalChannelNamespace = true
		if personalSingleConnection && !c.Channel.WithoutNamespace.Presence {
			return fmt.Errorf("presence must be enabled on top level to maintain single connection")
		}
	}

	nss := make([]string, 0, len(c.Channel.Namespaces))
	for _, n := range c.Channel.Namespaces {
		if slices.Contains(nss, n.Name) {
			return fmt.Errorf("namespace name must be unique: %s", n.Name)
		}
		if err := validateNamespace(n, c.Channel.HistoryMetaTTL, proxyNames, c); err != nil {
			return fmt.Errorf("namespace %s: %v", n.Name, err)
		}
		if n.Name == personalChannelNamespace {
			validPersonalChannelNamespace = true
			if personalSingleConnection && !n.Presence {
				return fmt.Errorf("presence must be enabled for namespace %s to maintain single connection", n.Name)
			}
		}
		nss = append(nss, n.Name)
	}

	if !validPersonalChannelNamespace {
		return fmt.Errorf("namespace for user personal channel not found: %s", personalChannelNamespace)
	}

	// Validate that map presence channel prefix references point to existing namespaces with map type.
	boundary := c.Channel.NamespaceBoundary
	if boundary == "" {
		boundary = ":"
	}
	for _, n := range c.Channel.Namespaces {
		if n.MapClientsPresenceChannelPrefix != "" {
			targetNs := strings.SplitN(n.MapClientsPresenceChannelPrefix+"x", boundary, 2)[0]
			if !slices.Contains(nss, targetNs) {
				return fmt.Errorf("namespace %s: map_clients_presence_channel_prefix %q resolves to namespace %q which does not exist", n.Name, n.MapClientsPresenceChannelPrefix, targetNs)
			}
		}
		if n.MapUsersPresenceChannelPrefix != "" {
			targetNs := strings.SplitN(n.MapUsersPresenceChannelPrefix+"x", boundary, 2)[0]
			if !slices.Contains(nss, targetNs) {
				return fmt.Errorf("namespace %s: map_users_presence_channel_prefix %q resolves to namespace %q which does not exist", n.Name, n.MapUsersPresenceChannelPrefix, targetNs)
			}
		}
	}

	rpcNss := make([]string, 0, len(c.RPC.Namespaces))
	for _, n := range c.RPC.Namespaces {
		if slices.Contains(rpcNss, n.Name) {
			return fmt.Errorf("rpc namespace name must be unique: %s", n.Name)
		}
		if err := validateRpcNamespace(n, proxyNames); err != nil {
			return fmt.Errorf("rpc namespace %s: %v", n.Name, err)
		}
		rpcNss = append(rpcNss, n.Name)
	}

	var consumerNames []string
	for _, config := range c.Consumers {
		if !consumerNameRe.Match([]byte(config.Name)) {
			return fmt.Errorf("invalid consumer name: %s, must match %s regular expression", config.Name, consumerNamePattern)
		}
		if slices.Contains(consumerNames, config.Name) {
			return fmt.Errorf("invalid consumer name: %s, must be unique", config.Name)
		}
		if !slices.Contains(configtypes.KnownConsumerTypes, config.Type) {
			return fmt.Errorf("unknown consumer type: %s", config.Type)
		}
		if config.Enabled {
			var err error
			switch config.Type {
			case configtypes.ConsumerTypeKafka:
				err = config.Kafka.Validate()
			case configtypes.ConsumerTypePostgres:
				err = config.Postgres.Validate()
			case configtypes.ConsumerTypeGooglePubSub:
				err = config.GooglePubSub.Validate()
			case configtypes.ConsumerTypeRedisStream:
				err = config.RedisStream.Validate()
			case configtypes.ConsumerTypeNatsJetStream:
				err = config.NatsJetStream.Validate()
			case configtypes.ConsumerTypeAwsSqs:
				err = config.AwsSqs.Validate()
			case configtypes.ConsumerTypeAzureServiceBus:
				err = config.AzureServiceBus.Validate()
			default:
			}
			if err != nil {
				return fmt.Errorf("in consumer %s (%s): %w", config.Name, config.Type, err)
			}
		}
		consumerNames = append(consumerNames, config.Name)
	}

	if err := validateConnectCodeTransforms(c.UniSSE.ConnectCodeToHTTPResponse.Transforms); err != nil {
		return fmt.Errorf("in uni_sse.connect_code_to_http_status.transforms: %v", err)
	}
	if err := validateConnectCodeTransforms(c.UniHTTPStream.ConnectCodeToHTTPResponse.Transforms); err != nil {
		return fmt.Errorf("in uni_http_stream.connect_code_to_http_status.transforms: %v", err)
	}

	// Map broker validation.
	var knownMapBrokers = []string{"memory", "redis", "postgres"}
	if !slices.Contains(knownMapBrokers, c.MapBroker.Type) {
		return fmt.Errorf("unknown map broker type: %s", c.MapBroker.Type)
	}

	return nil
}

var namePattern = "^[-a-zA-Z0-9_.]{2,}$"
var nameRe = regexp.MustCompile(namePattern)

func validateNamespace(ns configtypes.ChannelNamespace, globalHistoryMetaTTL configtypes.Duration, proxyNames []string, cfg Config) error {
	name := ns.Name
	match := nameRe.MatchString(name)
	if !match {
		return fmt.Errorf("invalid namespace name – %s (must match %s regular expression)", name, namePattern)
	}
	if err := validateChannelOptions(ns.ChannelOptions, globalHistoryMetaTTL, proxyNames, cfg); err != nil {
		return err
	}
	return nil
}

func validateRpcNamespace(ns configtypes.RpcNamespace, rpcProxyNames []string) error {
	name := ns.Name
	match := nameRe.MatchString(name)
	if !match {
		return fmt.Errorf("invalid rpc namespace name – %s (must match %s regular expression)", name, namePattern)
	}
	if err := validateRpcOptions(ns.RpcOptions, rpcProxyNames); err != nil {
		return err
	}
	return nil
}

func validateChannelOptions(c configtypes.ChannelOptions, globalHistoryMetaTTL configtypes.Duration, proxyNames []string, cfg Config) error {
	if err := validateSecondPrecisionDuration(c.HistoryTTL); err != nil {
		return fmt.Errorf("in history_ttl: %v", err)
	}
	if err := validateSecondPrecisionDuration(c.HistoryMetaTTL); err != nil {
		return fmt.Errorf("in history_meta_ttl: %v", err)
	}
	if (c.HistorySize != 0 && c.HistoryTTL == 0) || (c.HistorySize == 0 && c.HistoryTTL != 0) {
		return errors.New("both history size and history ttl required for history")
	}
	historyMetaTTL := globalHistoryMetaTTL
	if c.HistoryMetaTTL != 0 {
		historyMetaTTL = c.HistoryMetaTTL
	}
	if historyMetaTTL < c.HistoryTTL {
		return fmt.Errorf("history ttl (%s) can not be greater than history meta ttl (%s)", c.HistoryTTL, historyMetaTTL)
	}
	if c.ForceRecovery && (c.HistorySize == 0 || c.HistoryTTL == 0) {
		return errors.New("both history size and history ttl required for recovery")
	}
	if c.AutoCacheRecover && (!c.ForceRecovery || c.ForceRecoveryMode != "cache") {
		return errors.New("auto_cache_recover requires force_recovery and force_recovery_mode set to cache")
	}
	if c.ChannelRegex != "" {
		if _, err := regexp.Compile(c.ChannelRegex); err != nil {
			return fmt.Errorf("invalid channel regex %s: %w", c.ChannelRegex, err)
		}
	}
	if (c.SubscribeStreamProxyEnabled) && (c.SubscribeProxyEnabled || c.PublishProxyEnabled || c.SubRefreshProxyEnabled) {
		return fmt.Errorf("can't use subscribe stream proxy together with subscribe, publish or sub refresh proxies")
	}
	if len(c.AllowedDeltaTypes) > 0 {
		for _, dt := range c.AllowedDeltaTypes {
			if !slices.Contains([]centrifuge.DeltaType{centrifuge.DeltaTypeFossil}, dt) {
				return fmt.Errorf("unknown allowed delta type: \"%s\"", dt)
			}
		}
	}
	if !slices.Contains([]string{"", "stream", "cache"}, c.ForceRecoveryMode) {
		return fmt.Errorf("unknown recovery mode: \"%s\"", c.ForceRecoveryMode)
	}

	if !slices.Contains([]string{"", configtypes.PublicationDataFormatJSON, configtypes.PublicationDataFormatJSONObject, configtypes.PublicationDataFormatBinary}, c.PublicationDataFormat) {
		return fmt.Errorf("unknown publication_data_format: \"%s\"", c.PublicationDataFormat)
	}

	if c.SubscribeProxyName != "" && !slices.Contains(proxyNames, c.SubscribeProxyName) {
		return fmt.Errorf("subscribe proxy with name \"%s\" not found", c.SubscribeProxyName)
	}
	if c.SubscribeProxyEnabled && c.SubscribeProxyName == DefaultProxyName {
		if err := validateProxy("default", cfg.Channel.Proxy.Subscribe); err != nil {
			return fmt.Errorf("in channel.proxy.subscribe: %v", err)
		}
	}

	if c.PublishProxyName != "" && !slices.Contains(proxyNames, c.PublishProxyName) {
		return fmt.Errorf("publish proxy with name \"%s\" not found", c.PublishProxyName)
	}
	if c.PublishProxyEnabled && c.PublishProxyName == DefaultProxyName {
		if err := validateProxy("default", cfg.Channel.Proxy.Publish); err != nil {
			return fmt.Errorf("in channel.proxy.publish: %v", err)
		}
	}

	if c.SubRefreshProxyName != "" && !slices.Contains(proxyNames, c.SubRefreshProxyName) {
		return fmt.Errorf("sub refresh proxy with name \"%s\" not found", c.SubRefreshProxyName)
	}
	if c.SubRefreshProxyEnabled && c.SubRefreshProxyName == DefaultProxyName {
		if err := validateProxy("default", cfg.Channel.Proxy.SubRefresh); err != nil {
			return fmt.Errorf("in channel.proxy.sub_refresh: %v", err)
		}
	}

	if c.SubscribeStreamProxyName != "" && !slices.Contains(proxyNames, c.SubscribeStreamProxyName) {
		return fmt.Errorf("subscribe stream proxy with name \"%s\" not found", c.SubscribeStreamProxyName)
	}
	if c.SubscribeStreamProxyEnabled && c.SubscribeStreamProxyName == DefaultProxyName {
		if err := validateProxy("default", cfg.Channel.Proxy.SubscribeStream); err != nil {
			return fmt.Errorf("in channel.proxy.subscribe_stream: %v", err)
		}
	}

	if c.Map.PublishProxyName != "" && !slices.Contains(proxyNames, c.Map.PublishProxyName) {
		return fmt.Errorf("map publish proxy with name \"%s\" not found", c.Map.PublishProxyName)
	}
	if c.Map.PublishProxyEnabled && c.Map.PublishProxyName == DefaultProxyName {
		if err := validateProxy("default", cfg.Channel.Proxy.MapPublish); err != nil {
			return fmt.Errorf("in channel.proxy.map_publish: %v", err)
		}
	}

	if c.Map.RemoveProxyName != "" && !slices.Contains(proxyNames, c.Map.RemoveProxyName) {
		return fmt.Errorf("map remove proxy with name \"%s\" not found", c.Map.RemoveProxyName)
	}
	if c.Map.RemoveProxyEnabled && c.Map.RemoveProxyName == DefaultProxyName {
		if err := validateProxy("default", cfg.Channel.Proxy.MapRemove); err != nil {
			return fmt.Errorf("in channel.proxy.map_remove: %v", err)
		}
	}

	if c.Map.ClientKey != "" && !slices.Contains([]string{"client_id", "user_id"}, c.Map.ClientKey) {
		return fmt.Errorf("unknown map.client_key: %q (valid: \"client_id\", \"user_id\")", c.Map.ClientKey)
	}
	// map.client_key and the publish/remove proxy both decide the final key.
	// When a proxy is configured, it is solely responsible for keying (it may
	// echo the client's key or override). Allowing both at once silently lets
	// the proxy win over client_key, which surprises users who expect
	// client_key to force the keying. Reject the combination at config-load,
	// naming the actually-conflicting proxy field.
	if c.Map.ClientKey != "" && c.Map.PublishProxyEnabled {
		return fmt.Errorf("map.client_key is not compatible with map.publish_proxy_enabled: the proxy is responsible for keying when enabled")
	}
	if c.Map.ClientKey != "" && c.Map.RemoveProxyEnabled {
		return fmt.Errorf("map.client_key is not compatible with map.remove_proxy_enabled: the proxy is responsible for keying when enabled")
	}

	if c.SubscriptionType != "" && !slices.Contains([]string{"stream", "map", "map_clients", "map_users", "shared_poll"}, c.SubscriptionType) {
		return fmt.Errorf("unknown subscription_type: %q", c.SubscriptionType)
	}
	if c.SubscriptionType == "shared_poll" {
		if cfg.SharedPoll.HMACSecretKey == "" {
			return fmt.Errorf("shared_poll.hmac_secret_key is required when subscription_type is \"shared_poll\"")
		}
		if c.SharedPoll.ProxyName != "" && !slices.Contains(proxyNames, c.SharedPoll.ProxyName) {
			return fmt.Errorf("shared poll proxy with name %q not found", c.SharedPoll.ProxyName)
		}
		sharedPollProxyName := c.SharedPoll.ProxyName
		if sharedPollProxyName == "" {
			sharedPollProxyName = DefaultProxyName
		}
		if sharedPollProxyName == DefaultProxyName {
			if err := validateProxy("default", cfg.Channel.Proxy.SharedPollRefresh); err != nil {
				return fmt.Errorf("in channel.proxy.shared_poll_refresh: %v", err)
			}
		}
		if !slices.Contains([]string{"", "versionless", "versioned"}, c.SharedPoll.Mode) {
			return fmt.Errorf("invalid shared_poll mode: %q", c.SharedPoll.Mode)
		}
		if c.SharedPoll.PublishEnabled && (c.SharedPoll.Mode == "" || c.SharedPoll.Mode == "versionless") {
			return fmt.Errorf("shared_poll publish_enabled is incompatible with versionless mode (requires explicit versions)")
		}
	}
	if c.Map.Mode != "" && !slices.Contains([]string{"ephemeral", "recoverable", "persistent"}, c.Map.Mode) {
		return fmt.Errorf("unknown map.mode: %q (valid: \"ephemeral\", \"recoverable\", \"persistent\")", c.Map.Mode)
	}
	hasMapType := c.SubscriptionType == "map" || c.SubscriptionType == "map_clients" || c.SubscriptionType == "map_users"
	if hasMapType {
		if c.Map.Mode == "" {
			return fmt.Errorf("map.mode is required when subscription_type is a map type")
		}
	}
	mapHasExpiry := c.Map.Mode == "ephemeral" || c.Map.Mode == "recoverable"
	if mapHasExpiry {
		if c.Map.KeyTTL == 0 {
			return fmt.Errorf("map.key_ttl is required when map.mode is %q", c.Map.Mode)
		}
		if c.Map.KeyTTL.ToDuration() < 0 {
			return fmt.Errorf("map.key_ttl must be positive")
		}
	}
	if c.Map.Mode == "persistent" && c.Map.KeyTTL != 0 {
		return fmt.Errorf("map.key_ttl must not be set when map.mode is \"persistent\" (entries don't expire)")
	}
	// map_clients and map_users rely on TTL as the cleanup fallback: map_clients
	// uses MapRemove on disconnect with TTL as the safety net for transient
	// remove failures; map_users is *only* cleaned up by TTL (no remove on
	// disconnect by design). Persistent mode disables TTL, which means stale
	// presence entries can linger indefinitely. Refuse the combination.
	if c.Map.Mode == "persistent" && (c.SubscriptionType == "map_clients" || c.SubscriptionType == "map_users") {
		return fmt.Errorf("map.mode %q is not allowed with subscription_type %q (presence entries require TTL-based cleanup; use \"ephemeral\" or \"recoverable\")", c.Map.Mode, c.SubscriptionType)
	}
	if c.Map.Mode == "ephemeral" {
		if c.Map.StreamSize > 0 {
			return fmt.Errorf("map.stream_size must be 0 for map.mode \"ephemeral\"")
		}
		if c.Map.StreamTTL != 0 {
			return fmt.Errorf("map.stream_ttl must be 0 for map.mode \"ephemeral\"")
		}
		if c.Map.MetaTTL != 0 {
			return fmt.Errorf("map.meta_ttl must be 0 for map.mode \"ephemeral\"")
		}
	}
	mapHasStream := c.Map.Mode == "recoverable" || c.Map.Mode == "persistent"
	if mapHasStream {
		if c.Map.StreamSize < 0 {
			return fmt.Errorf("map.stream_size must be non-negative")
		}
		if c.Map.StreamTTL.ToDuration() < 0 {
			return fmt.Errorf("map.stream_ttl must be non-negative")
		}
		if c.Map.MetaTTL.ToDuration() < 0 {
			return fmt.Errorf("map.meta_ttl must be non-negative")
		}
		if c.Map.MetaTTL != 0 {
			effectiveStreamTTL := c.Map.StreamTTL.ToDuration()
			if effectiveStreamTTL == 0 {
				effectiveStreamTTL = time.Minute
			}
			if c.Map.MetaTTL.ToDuration() < effectiveStreamTTL {
				return fmt.Errorf("map.meta_ttl (%s) must be >= map.stream_ttl (%s) (metadata must outlive stream)", c.Map.MetaTTL, configtypes.Duration(effectiveStreamTTL))
			}
		}
	}
	if c.Map.RemoveClientOnUnsubscribe && !hasMapType {
		return fmt.Errorf("map.remove_client_on_unsubscribe requires subscription_type to be a map type")
	}

	// Map presence prefixes must resolve to a namespace whose subscription_type
	// is the matching map-presence flavour. Otherwise publishes to those
	// channels would silently fail at runtime.
	if c.MapClientsPresenceChannelPrefix != "" {
		if err := validatePresenceChannelPrefix(cfg, "map_clients_presence_channel_prefix", c.MapClientsPresenceChannelPrefix, "map_clients"); err != nil {
			return err
		}
	}
	if c.MapUsersPresenceChannelPrefix != "" {
		if err := validatePresenceChannelPrefix(cfg, "map_users_presence_channel_prefix", c.MapUsersPresenceChannelPrefix, "map_users"); err != nil {
			return err
		}
	}

	// Map page-size invariants. Zero means "use default" — the runtime path
	// fills in sensible values — but any non-zero combination must satisfy
	// min <= default <= max so clamping never produces surprising results.
	if c.Map.MinPageSize < 0 {
		return fmt.Errorf("map.min_page_size must not be negative")
	}
	if c.Map.MaxPageSize < 0 {
		return fmt.Errorf("map.max_page_size must not be negative")
	}
	if c.Map.DefaultPageSize < 0 {
		return fmt.Errorf("map.default_page_size must not be negative")
	}
	if c.Map.MinPageSize > 0 && c.Map.MaxPageSize > 0 && c.Map.MinPageSize > c.Map.MaxPageSize {
		return fmt.Errorf("map.min_page_size (%d) must not exceed map.max_page_size (%d)", c.Map.MinPageSize, c.Map.MaxPageSize)
	}
	if c.Map.DefaultPageSize > 0 && c.Map.MaxPageSize > 0 && c.Map.DefaultPageSize > c.Map.MaxPageSize {
		return fmt.Errorf("map.default_page_size (%d) must not exceed map.max_page_size (%d)", c.Map.DefaultPageSize, c.Map.MaxPageSize)
	}
	if c.Map.DefaultPageSize > 0 && c.Map.MinPageSize > 0 && c.Map.DefaultPageSize < c.Map.MinPageSize {
		return fmt.Errorf("map.default_page_size (%d) must not be below map.min_page_size (%d)", c.Map.DefaultPageSize, c.Map.MinPageSize)
	}

	return nil
}

// validatePresenceChannelPrefix checks that prefix, when prepended to a
// channel name, resolves under the configured NamespaceBoundary to a namespace
// whose subscription_type is `expectedType`. Caught at config-load so typos
// in the prefix don't surface only when the first map_publish fails.
func validatePresenceChannelPrefix(cfg Config, fieldName, prefix, expectedType string) error {
	boundary := cfg.Channel.NamespaceBoundary
	var nsName string
	if boundary != "" {
		idx := strings.Index(prefix, boundary)
		if idx >= 0 {
			nsName = prefix[:idx]
		}
	}
	opts, ok, err := channelOpts(&cfg, nsName)
	if err != nil {
		return fmt.Errorf("%s: %v", fieldName, err)
	}
	if !ok {
		return fmt.Errorf("%s %q: namespace %q does not exist", fieldName, prefix, nsName)
	}
	if opts.SubscriptionType != expectedType {
		return fmt.Errorf("%s %q: target namespace %q must have subscription_type=%q (got %q)", fieldName, prefix, nsName, expectedType, opts.SubscriptionType)
	}
	return nil
}

func validateRpcOptions(opts configtypes.RpcOptions, rpcProxyNames []string) error {
	if opts.ProxyName != "" && !slices.Contains(rpcProxyNames, opts.ProxyName) {
		return fmt.Errorf("proxy %s not found for rpc", opts.ProxyName)
	}
	return nil
}

var proxyNamePattern = "^[-a-zA-Z0-9_.]{2,}$"
var proxyNameRe = regexp.MustCompile(proxyNamePattern)

var consumerNamePattern = "^[a-zA-Z0-9_]{2,}$"
var consumerNameRe = regexp.MustCompile(consumerNamePattern)

func validateSecondPrecisionDuration(duration configtypes.Duration) error {
	if duration > 0 && duration.ToDuration()%time.Second != 0 {
		return fmt.Errorf("malformed duration %s, sub-second precision is not supported for this key", duration)
	}
	return nil
}

func validateProxy(name string, p configtypes.Proxy) error {
	if !proxyNameRe.MatchString(name) {
		return fmt.Errorf("invalid proxy name: %s, must match %s regular expression", name, proxyNamePattern)
	}
	if p.Timeout == 0 {
		return errors.New("timeout not set")
	}
	if p.Endpoint == "" {
		return errors.New("endpoint not set")
	}
	if err := validateStatusTransforms(p.ProxyCommon.HTTP.StatusToCodeTransforms); err != nil {
		return fmt.Errorf("in status_to_code_transforms: %v", err)
	}
	return nil
}

// Now Centrifugo uses https://github.com/tidwall/gjson to extract custom claims from JWT. So technically
// we could support extracting from nested objects using dot syntax, like "centrifugo.user". But for now
// not using this feature to keep things simple until necessary.
var customClaimRe = regexp.MustCompile("^[a-zA-Z_]+$")

func validateTokens(cfg Config) error {
	if cfg.Client.Token.UserIDClaim != "" {
		if !customClaimRe.MatchString(cfg.Client.Token.UserIDClaim) {
			return fmt.Errorf("invalid token custom user ID claim: %s, must match %s regular expression", cfg.Client.Token.UserIDClaim, customClaimRe.String())
		}
	}
	if cfg.Client.SubscriptionToken.UserIDClaim != "" {
		if !customClaimRe.MatchString(cfg.Client.SubscriptionToken.UserIDClaim) {
			return fmt.Errorf("invalid subscription token custom user ID claim: %s, must match %s regular expression", cfg.Client.SubscriptionToken.UserIDClaim, customClaimRe.String())
		}
	}
	return nil
}

func validateStatusTransforms(transforms []configtypes.HttpStatusToCodeTransform) error {
	for i, transform := range transforms {
		if transform.StatusCode == 0 {
			return fmt.Errorf("status_code should be set in status_to_code_transforms[%d]", i)
		}
		if transform.ToDisconnect.Code == 0 && transform.ToError.Code == 0 {
			return fmt.Errorf("no error or disconnect code set in status_to_code_transforms[%d]", i)
		}
		if transform.ToDisconnect.Code > 0 && transform.ToError.Code > 0 {
			return fmt.Errorf("only error or disconnect code can be set in status_to_code_transforms[%d], but not both", i)
		}
		if !tools.IsASCII(transform.ToDisconnect.Reason) {
			return fmt.Errorf("status_to_code_transforms[%d] disconnect reason must be ASCII", i)
		}
		if !tools.IsASCII(transform.ToError.Message) {
			return fmt.Errorf("status_to_code_transforms[%d] error message must be ASCII", i)
		}
		const reasonOrMessageMaxLength = 123 // limit comes from WebSocket close reason length limit. See https://datatracker.ietf.org/doc/html/rfc6455.
		if len(transform.ToError.Message) > reasonOrMessageMaxLength {
			return fmt.Errorf("status_to_code_transforms[%d] item error message can be up to %d characters long", i, reasonOrMessageMaxLength)
		}
		if len(transform.ToDisconnect.Reason) > reasonOrMessageMaxLength {
			return fmt.Errorf("status_to_code_transforms[%d] disconnect reason can be up to %d characters long", i, reasonOrMessageMaxLength)
		}
	}
	return nil
}

func validateConnectCodeTransforms(transforms []configtypes.ConnectCodeToHTTPResponseTransform) error {
	for i, transform := range transforms {
		if transform.Code == 0 {
			return fmt.Errorf("code should be set in connect_code_to_http_status.transforms[%d]", i)
		}
		if transform.To.StatusCode == 0 {
			return fmt.Errorf("status_code should be set in connect_code_to_http_status.transforms[%d].to_response", i)
		}
	}
	return nil
}

func validateCodeToUniDisconnectTransforms(transforms configtypes.UniConnectCodeToDisconnectTransforms) error {
	for i, t := range transforms {
		if t.Code == 0 {
			return fmt.Errorf("no code specified in transforms[%d]", i)
		}
		if t.To.Code == 0 {
			return fmt.Errorf("no disconnect code specified in transforms[%d].to", i)
		}
		if !tools.IsASCII(t.To.Reason) {
			return fmt.Errorf("disconnect reason must be ASCII in transforms[%d].to", i)
		}
	}
	return nil
}
