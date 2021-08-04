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
	// This should match one of configured namespace names. By default no namespace
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
	// ClientAnonymous when set to true, allows connect requests without specifying
	// a token or setting Credentials in authentication middleware. The resulting
	// user will have empty string for user ID, meaning user can only subscribe
	// to anonymous channels.
	ClientAnonymous bool
	// ClientConcurrency when set allows processing client commands concurrently
	// with provided concurrency level. By default commands processed sequentially
	// one after another.
	ClientConcurrency int
}

// DefaultConfig has default config options.
var DefaultConfig = Config{
	ChannelPrivatePrefix:     "$", // so private channel will look like "$gossips"
	ChannelNamespaceBoundary: ":", // so namespace "public" can be used as "public:news"
	ChannelUserBoundary:      "#", // so user limited channel is "user#2694" where "2696" is user ID
	ChannelUserSeparator:     ",", // so several users limited channel is "dialog#2694,3019"
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// Validate validates config and returns error if problems found
func (c *Config) Validate() error {
	pattern := "^[-a-zA-Z0-9_.]{2,}$"
	patternRegexp, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	if (c.HistorySize != 0 && c.HistoryTTL == 0) || (c.HistorySize == 0 && c.HistoryTTL != 0) {
		return errors.New("both history size and history ttl required for history")
	}

	if c.Recover && (c.HistorySize == 0 || c.HistoryTTL == 0) {
		return errors.New("both history size and history ttl required for history recovery")
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

	var nss = make([]string, 0, len(c.Namespaces))
	for _, n := range c.Namespaces {
		name := n.Name
		match := patternRegexp.MatchString(name)
		if !match {
			return fmt.Errorf("wrong namespace name – %s", name)
		}
		if stringInSlice(name, nss) {
			return fmt.Errorf("namespace name must be unique: %s", name)
		}
		if (n.HistorySize != 0 && n.HistoryTTL == 0) || (n.HistorySize == 0 && n.HistoryTTL != 0) {
			return fmt.Errorf("namespace %s: both history size and history ttl required for history", name)
		}
		if n.Recover && (n.HistorySize == 0 || n.HistoryTTL == 0) {
			return fmt.Errorf("namespace %s: both history size and history ttl required for history recovery", name)
		}
		if name == personalChannelNamespace {
			validPersonalChannelNamespace = true
			if personalSingleConnection && !n.Presence {
				return fmt.Errorf("presence must be enabled for namespace %s to maintain single connection", name)
			}
		}
		nss = append(nss, name)
	}

	if !validPersonalChannelNamespace {
		return fmt.Errorf("namespace for user personal channel not found: %s", personalChannelNamespace)
	}

	return nil
}

// Container ...
type Container struct {
	mu     sync.RWMutex
	config Config
}

// NewContainer ...
func NewContainer(config Config) *Container {
	return &Container{
		config: config,
	}
}

// Reload node config.
func (n *Container) Reload(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config = c
	return nil
}

// namespaceName returns namespace name from channel if exists.
func (n *Container) namespaceName(ch string) string {
	cTrim := strings.TrimPrefix(ch, n.config.ChannelPrivatePrefix)
	if n.config.ChannelNamespaceBoundary != "" && strings.Contains(cTrim, n.config.ChannelNamespaceBoundary) {
		parts := strings.SplitN(cTrim, n.config.ChannelNamespaceBoundary, 2)
		return parts[0]
	}
	return ""
}

// ChannelOptions returns channel options for channel using current channel config.
func (n *Container) ChannelOptions(ch string) (ChannelOptions, bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.channelOpts(n.namespaceName(ch))
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
