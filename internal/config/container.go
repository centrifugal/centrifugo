package config

import (
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"
)

// Container wraps configuration providing sync access, reload and compilation of required parts.
type Container struct {
	configValue            atomic.Value
	channelOptionsCache    *rollingCache
	ChannelOptionsCacheTTL time.Duration
}

const (
	channelOptionsCacheSize   = 8
	channelOptionsCacheShards = 128
)

// NewContainer creates new Container.
func NewContainer(config Config) (*Container, error) {
	preparedConfig, err := getPreparedConfig(config)
	if err != nil {
		return nil, err
	}
	c := &Container{
		channelOptionsCache: newRollingCache(channelOptionsCacheSize, channelOptionsCacheShards),
	}
	c.configValue.Store(*preparedConfig)
	return c, nil
}

func getPreparedConfig(config Config) (*Config, error) {
	if err := config.Validate(); err != nil {
		return &config, err
	}
	config, err := buildCompiledRegexes(config)
	if err != nil {
		return &config, err
	}
	return &config, nil
}

// Reload node config.
func (n *Container) Reload(c Config) error {
	preparedConfig, err := getPreparedConfig(c)
	if err != nil {
		return err
	}
	n.configValue.Store(*preparedConfig)
	return nil
}

func buildCompiledRegexes(config Config) (Config, error) {
	if config.Channel.WithoutNamespace.ChannelRegex != "" {
		p, err := regexp.Compile(config.Channel.WithoutNamespace.ChannelRegex)
		if err != nil {
			return config, err
		}
		config.Channel.WithoutNamespace.Compiled.CompiledChannelRegex = p
	}

	var namespaces []configtypes.ChannelNamespace
	for _, ns := range config.Channel.Namespaces {
		if ns.ChannelRegex == "" {
			namespaces = append(namespaces, ns)
			continue
		}
		p, err := regexp.Compile(ns.ChannelRegex)
		if err != nil {
			return config, err
		}
		ns.Compiled.CompiledChannelRegex = p
		namespaces = append(namespaces, ns)
	}
	config.Channel.Namespaces = namespaces
	return config, nil
}

// namespaceName returns namespace name from channel if exists.
func (n *Container) namespaceName(config Config, ch string) (string, string) {
	cTrim := strings.TrimPrefix(ch, config.Channel.PrivatePrefix)
	if config.Channel.NamespaceBoundary != "" && strings.Contains(cTrim, config.Channel.NamespaceBoundary) {
		parts := strings.SplitN(cTrim, config.Channel.NamespaceBoundary, 2)
		return parts[0], parts[1]
	}
	return "", ch
}

type channelOptionsResult struct {
	nsName string
	rest   string
	chOpts configtypes.ChannelOptions
	ok     bool
	err    error
}

// ChannelOptions returns channel options for channel using current channel config.
func (n *Container) ChannelOptions(ch string) (string, string, configtypes.ChannelOptions, bool, error) {
	if n.ChannelOptionsCacheTTL > 0 {
		if res, ok := n.channelOptionsCache.Get(ch); ok {
			return res.nsName, res.rest, res.chOpts, res.ok, res.err
		}
	}
	cfg := n.configValue.Load().(Config)
	nsName, rest := n.namespaceName(cfg, ch)
	chOpts, ok, err := channelOpts(&cfg, nsName)

	res := channelOptionsResult{
		nsName: nsName,
		rest:   rest,
		chOpts: chOpts,
		ok:     ok,
		err:    err,
	}
	if n.ChannelOptionsCacheTTL > 0 {
		n.channelOptionsCache.Set(ch, res, n.ChannelOptionsCacheTTL)
	}
	return res.nsName, res.rest, res.chOpts, res.ok, res.err
}

// NumNamespaces returns number of configured namespaces.
func (n *Container) NumNamespaces() int {
	c := n.configValue.Load().(Config)
	return len(c.Channel.Namespaces)
}

// channelOpts searches for channel options for specified namespace key.
func channelOpts(c *Config, namespaceName string) (configtypes.ChannelOptions, bool, error) {
	if namespaceName == "" {
		return c.Channel.WithoutNamespace, true, nil
	}
	for _, n := range c.Channel.Namespaces {
		if n.Name == namespaceName {
			return n.ChannelOptions, true, nil
		}
	}
	return configtypes.ChannelOptions{}, false, nil
}

// PersonalChannel returns personal channel for user based on node configuration.
func (n *Container) PersonalChannel(user string) string {
	cfg := n.Config()
	if cfg.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace == "" {
		return cfg.Channel.UserBoundary + user
	}
	return cfg.Client.SubscribeToUserPersonalChannel.PersonalChannelNamespace + cfg.Channel.NamespaceBoundary + cfg.Channel.UserBoundary + user
}

// Config returns a copy of node Config.
func (n *Container) Config() Config {
	return n.configValue.Load().(Config)
}

// IsPrivateChannel checks if channel requires token to subscribe. In case of
// token-protected channel subscription request must contain a proper token.
func (n *Container) IsPrivateChannel(ch string) bool {
	cfg := n.configValue.Load().(Config)
	if cfg.Channel.PrivatePrefix == "" {
		return false
	}
	return strings.HasPrefix(ch, cfg.Channel.PrivatePrefix)
}

// IsUserLimited returns whether channel is user-limited.
func (n *Container) IsUserLimited(ch string) bool {
	cfg := n.configValue.Load().(Config)
	userBoundary := cfg.Channel.UserBoundary
	if userBoundary == "" {
		return false
	}
	return strings.Contains(ch, userBoundary)
}

// UserAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (n *Container) UserAllowed(ch string, user string) bool {
	cfg := n.configValue.Load().(Config)
	userBoundary := cfg.Channel.UserBoundary
	userSeparator := cfg.Channel.UserSeparator
	if userBoundary == "" {
		return true
	}
	if !strings.Contains(ch, userBoundary) {
		return true
	}
	parts := strings.Split(ch, userBoundary)
	if userSeparator == "" {
		return parts[len(parts)-1] == user
	}
	allowedUsers := strings.Split(parts[len(parts)-1], userSeparator)
	for _, allowedUser := range allowedUsers {
		if user == allowedUser {
			return true
		}
	}
	return false
}

// rpcNamespaceName returns rpc namespace name from channel if exists.
func (n *Container) rpcNamespaceName(method string) string {
	cfg := n.configValue.Load().(Config)
	if cfg.RPC.NamespaceBoundary != "" && strings.Contains(method, cfg.RPC.NamespaceBoundary) {
		parts := strings.SplitN(method, cfg.RPC.NamespaceBoundary, 2)
		return parts[0]
	}
	return ""
}

// RpcOptions returns rpc options for method using current config.
func (n *Container) RpcOptions(method string) (configtypes.RpcOptions, bool, error) {
	cfg := n.configValue.Load().(Config)
	return rpcOpts(&cfg, n.rpcNamespaceName(method))
}

// NumRpcNamespaces returns number of configured rpc namespaces.
func (n *Container) NumRpcNamespaces() int {
	cfg := n.configValue.Load().(Config)
	return len(cfg.RPC.Namespaces)
}

// rpcOpts searches for channel options for specified namespace key.
func rpcOpts(c *Config, namespaceName string) (configtypes.RpcOptions, bool, error) {
	if namespaceName == "" {
		return c.RPC.WithoutNamespace, true, nil
	}
	for _, n := range c.RPC.Namespaces {
		if n.Name == namespaceName {
			return n.RpcOptions, true, nil
		}
	}
	return configtypes.RpcOptions{}, false, nil
}
