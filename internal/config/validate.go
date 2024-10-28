package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/configtypes"
	"github.com/centrifugal/centrifugo/v5/internal/tools"

	"github.com/centrifugal/centrifuge"
)

// Validate validates config and returns error if problems found.
func (c Config) Validate() error {
	if c.Broker.Type != "memory" && c.Broker.Type != "nats" && c.Broker.Type != "redis" && c.Broker.Type != "redisnats" {
		return fmt.Errorf("unknown broker: %s", c.Broker.Type)
	}

	if err := validateTokens(c); err != nil {
		return err
	}

	var proxyNames []string
	for _, p := range c.Proxies {
		if !proxyNameRe.Match([]byte(p.Name)) {
			return fmt.Errorf("invalid proxy name: %s, must match %s regular expression", p.Name, proxyNamePattern)
		}
		if slices.Contains(proxyNames, p.Name) {
			return fmt.Errorf("duplicate proxy name: %s", p.Name)
		}
		if p.Timeout == 0 {
			p.Timeout = configtypes.Duration(time.Second)
		}
		if p.Endpoint == "" {
			return fmt.Errorf("no endpoint set for proxy %s", p.Name)
		}
		if err := validateStatusTransforms(p.ProxyCommon.HTTP.StatusToCodeTransforms); err != nil {
			return fmt.Errorf("in proxy %s: %v", p.Name, err)
		}

		proxyNames = append(proxyNames, p.Name)
	}
	if slices.Contains(proxyNames, UnifiedProxyName) {
		return fmt.Errorf("proxy name %s is reserved, it's a name of proxy configured over unified_proxy option", UnifiedProxyName)
	}

	proxyNames = append(proxyNames, UnifiedProxyName) // channel options can use global proxy name.

	if c.Client.ConnectProxyName != "" && !slices.Contains(proxyNames, c.Client.ConnectProxyName) {
		return fmt.Errorf("proxy %s not found for connect", c.Client.ConnectProxyName)
	}
	if c.Client.RefreshProxyName != "" && !slices.Contains(proxyNames, c.Client.RefreshProxyName) {
		return fmt.Errorf("proxy %s not found for refresh", c.Client.RefreshProxyName)
	}
	if c.Client.ConnectProxyName == UnifiedProxyName && c.UnifiedProxy.ConnectEndpoint == "" {
		return fmt.Errorf("no connect_endpoint set for unified_proxy, can't use `%s` proxy name for client connect proxy", UnifiedProxyName)
	}
	if c.Client.RefreshProxyName == UnifiedProxyName && c.UnifiedProxy.RefreshEndpoint == "" {
		return fmt.Errorf("no refresh_endpoint set for unified_proxy, can't use `%s` proxy name for client refresh proxy", UnifiedProxyName)
	}
	if err := validateSecondPrecisionDuration(c.Channel.HistoryMetaTTL); err != nil {
		return fmt.Errorf("in channel.history_meta_ttl: %v", err)
	}
	if err := validateStatusTransforms(c.UnifiedProxy.ProxyCommon.HTTP.StatusToCodeTransforms); err != nil {
		return fmt.Errorf("in proxy %s: %v", UnifiedProxyName, err)
	}

	if err := validateChannelOptions(c.Channel.WithoutNamespace, c.Channel.HistoryMetaTTL, proxyNames, c); err != nil {
		return fmt.Errorf("in channel.without_namespace: %v", err)
	}
	if err := validateRpcOptions(c.RPC.WithoutNamespace); err != nil {
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

	rpcNss := make([]string, 0, len(c.RPC.Namespaces))
	for _, n := range c.RPC.Namespaces {
		if slices.Contains(rpcNss, n.Name) {
			return fmt.Errorf("rpc namespace name must be unique: %s", n.Name)
		}
		if err := validateRpcNamespace(n); err != nil {
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
		consumerNames = append(consumerNames, config.Name)
	}

	if err := validateConnectCodeTransforms(c.UniSSE.ConnectCodeToHTTPStatus.Transforms); err != nil {
		return fmt.Errorf("in uni_sse.connect_code_to_http_status.transforms: %v", err)
	}
	if err := validateConnectCodeTransforms(c.UniHTTPStream.ConnectCodeToHTTPStatus.Transforms); err != nil {
		return fmt.Errorf("in uni_http_stream.connect_code_to_http_status.transforms: %v", err)
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

func validateRpcNamespace(ns configtypes.RpcNamespace) error {
	name := ns.Name
	match := nameRe.MatchString(name)
	if !match {
		return fmt.Errorf("invalid rpc namespace name – %s (must match %s regular expression)", name, namePattern)
	}
	if err := validateRpcOptions(ns.RpcOptions); err != nil {
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
	if c.ChannelRegex != "" {
		if _, err := regexp.Compile(c.ChannelRegex); err != nil {
			return fmt.Errorf("invalid channel regex %s: %w", c.ChannelRegex, err)
		}
	}
	if (c.SubscribeStreamProxyName != "") && (c.SubscribeProxyName != "" || c.PublishProxyName != "" || c.SubRefreshProxyName != "") {
		return fmt.Errorf("can't use subscribe stream proxy together with subscribe, publish or sub refresh proxies")
	}
	if len(c.AllowedDeltaTypes) > 0 {
		for _, dt := range c.AllowedDeltaTypes {
			if !slices.Contains([]centrifuge.DeltaType{centrifuge.DeltaTypeFossil}, dt) {
				return fmt.Errorf("unknown allowed delta type: %s", dt)
			}
		}
	}
	if !slices.Contains([]string{"", "stream", "cache"}, c.ForceRecoveryMode) {
		return fmt.Errorf("unknown recovery mode: %s", c.ForceRecoveryMode)
	}
	if c.SubscribeProxyName != "" && !slices.Contains(proxyNames, c.SubscribeProxyName) {
		return fmt.Errorf("proxy %s not found for subscribe", c.SubscribeProxyName)
	}
	if c.PublishProxyName != "" && !slices.Contains(proxyNames, c.PublishProxyName) {
		return fmt.Errorf("proxy %s not found for publish", c.PublishProxyName)
	}
	if c.SubRefreshProxyName != "" && !slices.Contains(proxyNames, c.SubRefreshProxyName) {
		return fmt.Errorf("proxy %s not found for sub refresh", c.SubRefreshProxyName)
	}
	if c.SubscribeStreamProxyName != "" && !slices.Contains(proxyNames, c.SubscribeStreamProxyName) {
		return fmt.Errorf("proxy %s not found for subscribe stream", c.SubscribeStreamProxyName)
	}

	if c.SubscribeProxyName == UnifiedProxyName && cfg.UnifiedProxy.SubscribeEndpoint == "" {
		return fmt.Errorf("no subscribe_endpoint set for unified_proxy, can't use `%s` proxy name for subscribe proxy", UnifiedProxyName)
	}
	if c.PublishProxyName == UnifiedProxyName && cfg.UnifiedProxy.PublishEndpoint == "" {
		return fmt.Errorf("no publish_endpoint set for unified_proxy, can't use `%s` proxy name for publish proxy", UnifiedProxyName)
	}
	if c.SubRefreshProxyName == UnifiedProxyName && cfg.UnifiedProxy.SubRefreshEndpoint == "" {
		return fmt.Errorf("no sub_refresh_endpoint set for unified_proxy, can't use `%s` proxy name for sub refresh proxy", UnifiedProxyName)
	}
	if c.SubscribeStreamProxyName == UnifiedProxyName && cfg.UnifiedProxy.SubscribeStreamEndpoint == "" {
		return fmt.Errorf("no subscribe_stream_endpoint set for unified_proxy, can't use `%s` proxy name for subscribe stream proxy", UnifiedProxyName)
	}

	return nil
}

func validateRpcOptions(_ configtypes.RpcOptions) error {
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

func validateConnectCodeTransforms(transforms []configtypes.ConnectCodeToHTTPStatusTransform) error {
	for i, transform := range transforms {
		if transform.Code == 0 {
			return fmt.Errorf("code should be set in connect_code_to_http_status.transforms[%d]", i)
		}
		if transform.ToResponse.StatusCode == 0 {
			return fmt.Errorf("status_code should be set in connect_code_to_http_status.transforms[%d].to_response", i)
		}
	}
	return nil
}
