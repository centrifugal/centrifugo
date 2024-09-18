package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/centrifugal/centrifugo/v5/internal/configtypes"

	"github.com/centrifugal/centrifuge"
)

// Validate validates config and returns error if problems found
func (c Config) Validate() error {
	if c.Broker != "" && c.Broker != "nats" {
		return fmt.Errorf("unknown broker: %s", c.Broker)
	}

	if err := validateToken(c); err != nil {
		return err
	}

	if err := validateChannelOptions(c.Channel.WithoutNamespace, c.GlobalHistoryMetaTTL); err != nil {
		return err
	}
	if err := validateRpcOptions(c.RPC.WithoutNamespace); err != nil {
		return err
	}

	usePersonalChannel := c.UserSubscribeToPersonal.Enabled
	personalChannelNamespace := c.UserSubscribeToPersonal.PersonalChannelNamespace
	personalSingleConnection := c.UserSubscribeToPersonal.SingleConnection
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
		if err := validateNamespace(n, c.GlobalHistoryMetaTTL); err != nil {
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

	proxyNames := map[string]struct{}{}
	for _, p := range c.Proxies {
		if !proxyNameRe.Match([]byte(p.Name)) {
			return fmt.Errorf("invalid proxy name: %s, must match %s regular expression", p.Name, proxyNamePattern)
		}
		if _, ok := proxyNames[p.Name]; ok {
			return fmt.Errorf("duplicate proxy name: %s", p.Name)
		}
		if p.Timeout == 0 {
			p.Timeout = time.Second
		}
		if p.Endpoint == "" {
			return fmt.Errorf("no endpoint set for proxy %s", p.Name)
		}
		proxyNames[p.Name] = struct{}{}
	}

	var consumerNames []string
	for _, config := range c.Consumers {
		if !consumerNameRe.Match([]byte(config.Name)) {
			return fmt.Errorf("invalid consumer name: %s, must match %s regular expression", config.Name, consumerNamePattern)
		}
		if slices.Contains(consumerNames, config.Name) {
			return fmt.Errorf("invalid consumer name: %s, must be unique", config.Name)
		}
		consumerNames = append(consumerNames, config.Name)
	}

	return nil
}

var namePattern = "^[-a-zA-Z0-9_.]{2,}$"
var nameRe = regexp.MustCompile(namePattern)

func validateNamespace(ns configtypes.ChannelNamespace, globalHistoryMetaTTL time.Duration) error {
	name := ns.Name
	match := nameRe.MatchString(name)
	if !match {
		return fmt.Errorf("invalid namespace name – %s (must match %s regular expression)", name, namePattern)
	}
	if err := validateChannelOptions(ns.ChannelOptions, globalHistoryMetaTTL); err != nil {
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

func validateChannelOptions(c configtypes.ChannelOptions, globalHistoryMetaTTL time.Duration) error {
	if (c.HistorySize != 0 && c.HistoryTTL == 0) || (c.HistorySize == 0 && c.HistoryTTL != 0) {
		return errors.New("both history size and history ttl required for history")
	}
	historyMetaTTL := globalHistoryMetaTTL
	if c.HistoryMetaTTL != 0 {
		historyMetaTTL = time.Duration(c.HistoryMetaTTL)
	}
	if historyMetaTTL < time.Duration(c.HistoryTTL) {
		return fmt.Errorf("history ttl (%s) can not be greater than history meta ttl (%s)", time.Duration(c.HistoryTTL), historyMetaTTL)
	}
	if c.ForceRecovery && (c.HistorySize == 0 || c.HistoryTTL == 0) {
		return errors.New("both history size and history ttl required for recovery")
	}
	if c.ChannelRegex != "" {
		if _, err := regexp.Compile(c.ChannelRegex); err != nil {
			return fmt.Errorf("invalid channel regex %s: %w", c.ChannelRegex, err)
		}
	}
	if (c.ProxySubscribeStream || c.SubscribeStreamProxyName != "") && (c.ProxySubscribe || c.ProxyPublish || c.ProxySubRefresh) {
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
	return nil
}

func validateRpcOptions(_ configtypes.RpcOptions) error {
	return nil
}

var proxyNamePattern = "^[-a-zA-Z0-9_.]{2,}$"
var proxyNameRe = regexp.MustCompile(proxyNamePattern)

var consumerNamePattern = "^[a-zA-Z0-9_]{2,}$"
var consumerNameRe = regexp.MustCompile(consumerNamePattern)

//func validateDuration(duration time.Duration) time.Duration {
//	// TODO: v6 – move to config.
//	if duration > 0 && duration < time.Millisecond {
//		log.Fatal().Msgf("malformed duration %s, minimal duration resolution is 1ms – make sure correct time unit set", duration)
//	}
//	if duration > 0 && duration < time.Second {
//		log.Fatal().Msgf("malformed duration %s, minimal duration resolution is 1s for this key", duration)
//	}
//	if duration > 0 && duration%time.Second != 0 {
//		log.Fatal().Msgf("malformed duration %s, sub-second precision is not supported for this key", duration)
//	}
//	return duration
//}

// Now Centrifugo uses https://github.com/tidwall/gjson to extract custom claims from JWT. So technically
// we could support extracting from nested objects using dot syntax, like "centrifugo.user". But for now
// not using this feature to keep things simple until necessary.
var customClaimRe = regexp.MustCompile("^[a-zA-Z_]+$")

func validateToken(cfg Config) error {
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
