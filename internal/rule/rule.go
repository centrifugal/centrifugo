package rule

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Config ...
type Config struct {
	// ChannelOptions embedded on top level.
	ChannelOptions
	// Namespaces – list of namespaces for custom channel options.
	Namespaces []ChannelNamespace
	// RpcOptions embedded on top level.
	RpcOptions
	// RpcNamespaces - list of rpc namespace for custom rpc options.
	RpcNamespaces []RpcNamespace
	// RpcNamespaceBoundary is a string separator which must be put after
	// rpc namespace part in rpc method.
	RpcNamespaceBoundary string
	// ChannelUserBoundary is a string separator which must be set before
	// allowed users part in channel name.
	// ChannelPrivatePrefix is a prefix in channel name which indicates that
	// channel is private.
	ChannelPrivatePrefix string
	// ChannelNamespaceBoundary is a string separator which must be put after
	// namespace part in channel name.
	ChannelNamespaceBoundary string
	// ChannelUserBoundary is a string separator which must be set before
	// allowed users part in channel name.
	ChannelUserBoundary string
	// ChannelUserSeparator separates allowed users in user part of channel name.
	// So you can limit access to channel to limited set of users.
	ChannelUserSeparator string
	// UserSubscribeToPersonal enables automatic subscribing to personal channel
	// by user.  Only users with user ID defined will subscribe to personal
	// channels, anonymous users are ignored.
	UserSubscribeToPersonal bool
	// UserPersonalChannelPrefix defines prefix to be added to user personal channel.
	// This should match one of configured namespace names. By default, no namespace
	// used for personal channel.
	UserPersonalChannelNamespace string
	// UserPersonalSingleConnection turns on a mode in which Centrifugo will try to
	// maintain only a single connection for each user in the same moment. As soon as
	// user establishes a connection other connections from the same user will be closed
	// with connection limit reason.
	// This feature works with a help of presence information inside personal channel.
	// So presence should be turned on in personal channel.
	UserPersonalSingleConnection bool
	// ClientInsecure turns on insecure mode for client connections - when it's
	// turned on then no authentication required at all when connecting to Centrifugo,
	// anonymous access and publish allowed for all channels, no connection expire
	// performed. This can be suitable for demonstration or personal usage.
	ClientInsecure bool
	// AnonymousConnectWithoutToken when set to true, allows connecting without specifying
	// a connection token or setting Credentials in authentication middleware. The resulting
	// user will have empty string for user ID (i.e. user is treated as anonymous).
	AnonymousConnectWithoutToken bool

	// DisallowAnonymousConnectionTokens tells Centrifugo to not accept connections from
	// anonymous users even if they provided a valid JWT. I.e. if token is valid but `sub`
	// claim is empty then Centrifugo closes connection with advice to not reconnect again.
	DisallowAnonymousConnectionTokens bool

	// ClientConcurrency when set allows processing client commands concurrently
	// with provided concurrency level. By default, commands processed sequentially
	// one after another.
	ClientConcurrency int
	// ClientConnectionLimit sets the maximum number of concurrent clients a single Centrifugo
	// node will accept.
	ClientConnectionLimit int
	// ClientConnectionRateLimit sets the maximum number of new connections a single Centrifugo
	// node will accept per second.
	ClientConnectionRateLimit int

	// ProxyConnectionToken bool
	ProxyConnectionToken bool
}

// DefaultConfig has default config options.
var DefaultConfig = Config{
	ChannelPrivatePrefix:     "$", // so private channel will look like "$gossips".
	ChannelNamespaceBoundary: ":", // so namespace "public" can be used as "public:news".
	ChannelUserBoundary:      "#", // so user limited channel is "user#2694" where "2696" is user ID.
	ChannelUserSeparator:     ",", // so several users limited channel is "dialog#2694,3019".
	RpcNamespaceBoundary:     ":", // so rpc namespace "chat" can be used as "chat:get_user_info".
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

var namePattern = "^[-a-zA-Z0-9_.]{2,}$"
var nameRe = regexp.MustCompile(namePattern)

func ValidateNamespace(ns ChannelNamespace) error {
	name := ns.Name
	match := nameRe.MatchString(name)
	if !match {
		return fmt.Errorf("invalid namespace name – %s (must match %s regular expression)", name, namePattern)
	}
	if err := ValidateChannelOptions(ns.ChannelOptions); err != nil {
		return err
	}
	return nil
}

func ValidateRpcNamespace(ns RpcNamespace) error {
	name := ns.Name
	match := nameRe.MatchString(name)
	if !match {
		return fmt.Errorf("invalid rpc namespace name – %s (must match %s regular expression)", name, namePattern)
	}
	if err := ValidateRpcOptions(ns.RpcOptions); err != nil {
		return err
	}
	return nil
}

func ValidateChannelOptions(c ChannelOptions) error {
	if (c.HistorySize != 0 && c.HistoryTTL == 0) || (c.HistorySize == 0 && c.HistoryTTL != 0) {
		return errors.New("both history size and history ttl required for history")
	}
	if c.ForceRecovery && (c.HistorySize == 0 || c.HistoryTTL == 0) {
		return errors.New("both history size and history ttl required for recovery")
	}
	if c.ChannelRegex != "" {
		if _, err := regexp.Compile(c.ChannelRegex); err != nil {
			return fmt.Errorf("invalid channel regex %s: %w", c.ChannelRegex, err)
		}
	}
	return nil
}

func ValidateRpcOptions(_ RpcOptions) error {
	return nil
}

// Validate validates config and returns error if problems found
func (c *Config) Validate() error {
	if err := ValidateChannelOptions(c.ChannelOptions); err != nil {
		return err
	}
	if err := ValidateRpcOptions(c.RpcOptions); err != nil {
		return err
	}

	usePersonalChannel := c.UserSubscribeToPersonal
	personalChannelNamespace := c.UserPersonalChannelNamespace
	personalSingleConnection := c.UserPersonalSingleConnection
	var validPersonalChannelNamespace bool
	if !usePersonalChannel || personalChannelNamespace == "" {
		validPersonalChannelNamespace = true
		if personalSingleConnection && !c.Presence {
			return fmt.Errorf("presence must be enabled on top level to maintain single connection")
		}
	}

	nss := make([]string, 0, len(c.Namespaces))
	for _, n := range c.Namespaces {
		if stringInSlice(n.Name, nss) {
			return fmt.Errorf("namespace name must be unique: %s", n.Name)
		}
		if err := ValidateNamespace(n); err != nil {
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

	rpcNss := make([]string, 0, len(c.RpcNamespaces))
	for _, n := range c.RpcNamespaces {
		if stringInSlice(n.Name, rpcNss) {
			return fmt.Errorf("rpc namespace name must be unique: %s", n.Name)
		}
		if err := ValidateRpcNamespace(n); err != nil {
			return fmt.Errorf("rpc namespace %s: %v", n.Name, err)
		}
		rpcNss = append(rpcNss, n.Name)
	}

	return nil
}

// Container ...
type Container struct {
	mu     sync.RWMutex
	config Config
}

// NewContainer ...
func NewContainer(config Config) (*Container, error) {
	preparedConfig, err := getPreparedConfig(config)
	if err != nil {
		return nil, err
	}
	return &Container{
		config: *preparedConfig,
	}, nil
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
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config = *preparedConfig
	return nil
}

func buildCompiledRegexes(config Config) (Config, error) {

	if config.ChannelRegex != "" {
		p, err := regexp.Compile(config.ChannelRegex)
		if err != nil {
			return config, err
		}
		config.Compiled.CompiledChannelRegex = p
	}

	for _, ns := range config.Namespaces {
		if ns.ChannelRegex == "" {
			continue
		}
		p, err := regexp.Compile(ns.ChannelRegex)
		if err != nil {
			return config, err
		}
		ns.Compiled.CompiledChannelRegex = p
	}

	return config, nil
}

// namespaceName returns namespace name from channel if exists.
func (n *Container) namespaceName(ch string) (string, string) {
	cTrim := strings.TrimPrefix(ch, n.config.ChannelPrivatePrefix)
	if n.config.ChannelNamespaceBoundary != "" && strings.Contains(cTrim, n.config.ChannelNamespaceBoundary) {
		parts := strings.SplitN(cTrim, n.config.ChannelNamespaceBoundary, 2)
		return parts[0], parts[1]
	}
	return "", ch
}

// ChannelOptions returns channel options for channel using current channel config.
func (n *Container) ChannelOptions(ch string) (string, string, ChannelOptions, bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	nsName, rest := n.namespaceName(ch)
	chOpts, ok, err := n.config.channelOpts(nsName)
	return nsName, rest, chOpts, ok, err
}

// NumNamespaces returns number of configured namespaces.
func (n *Container) NumNamespaces() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.config.Namespaces)
}

// channelOpts searches for channel options for specified namespace key.
func (c *Config) channelOpts(namespaceName string) (ChannelOptions, bool, error) {
	if namespaceName == "" {
		return c.ChannelOptions, true, nil
	}
	for _, n := range c.Namespaces {
		if n.Name == namespaceName {
			return n.ChannelOptions, true, nil
		}
	}
	return ChannelOptions{}, false, nil
}

// PersonalChannel returns personal channel for user based on node configuration.
func (n *Container) PersonalChannel(user string) string {
	config := n.Config()
	if config.UserPersonalChannelNamespace == "" {
		return config.ChannelUserBoundary + user
	}
	return config.UserPersonalChannelNamespace + config.ChannelNamespaceBoundary + config.ChannelUserBoundary + user
}

// Config returns a copy of node Config.
func (n *Container) Config() Config {
	n.mu.RLock()
	c := n.config
	n.mu.RUnlock()
	return c
}

// IsPrivateChannel checks if channel requires token to subscribe. In case of
// token-protected channel subscription request must contain a proper token.
func (n *Container) IsPrivateChannel(ch string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.config.ChannelPrivatePrefix == "" {
		return false
	}
	return strings.HasPrefix(ch, n.config.ChannelPrivatePrefix)
}

// IsUserLimited returns whether channel is user-limited.
func (n *Container) IsUserLimited(ch string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	userBoundary := n.config.ChannelUserBoundary
	if userBoundary == "" {
		return false
	}
	return strings.Contains(ch, userBoundary)
}

// UserAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (n *Container) UserAllowed(ch string, user string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	userBoundary := n.config.ChannelUserBoundary
	userSeparator := n.config.ChannelUserSeparator
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
	if n.config.RpcNamespaceBoundary != "" && strings.Contains(method, n.config.RpcNamespaceBoundary) {
		parts := strings.SplitN(method, n.config.RpcNamespaceBoundary, 2)
		return parts[0]
	}
	return ""
}

// RpcOptions returns rpc options for method using current config.
func (n *Container) RpcOptions(method string) (RpcOptions, bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.rpcOpts(n.rpcNamespaceName(method))
}

// NumRpcNamespaces returns number of configured rpc namespaces.
func (n *Container) NumRpcNamespaces() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.config.RpcNamespaces)
}

// rpcOpts searches for channel options for specified namespace key.
func (c *Config) rpcOpts(namespaceName string) (RpcOptions, bool, error) {
	if namespaceName == "" {
		return c.RpcOptions, true, nil
	}
	for _, n := range c.RpcNamespaces {
		if n.Name == namespaceName {
			return n.RpcOptions, true, nil
		}
	}
	return RpcOptions{}, false, nil
}
