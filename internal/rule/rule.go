package rule

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/centrifugal/centrifuge"
)

type ChannelRuleConfig struct {
	// NamespaceChannelOptions embedded on top level.
	NamespaceChannelOptions
	// Namespaces – list of namespaces for custom channel options.
	Namespaces []ChannelNamespace
	// TokenChannelPrefix is a prefix in channel name which indicates that
	// channel is private.
	TokenChannelPrefix string
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
}

// DefaultRuleConfig has default config options.
var DefaultRuleConfig = ChannelRuleConfig{
	TokenChannelPrefix:       "$", // so private channel will look like "$gossips"
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
func (c *ChannelRuleConfig) Validate() error {
	errPrefix := "config error: "
	pattern := "^[-a-zA-Z0-9_.]{2,}$"
	patternRegexp, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	if c.HistoryRecover && (c.HistorySize == 0 || c.HistoryLifetime == 0) {
		return errors.New("both history size and history lifetime required for history recovery")
	}

	usePersonalChannel := c.UserSubscribeToPersonal
	personalChannelNamespace := c.UserPersonalChannelNamespace
	var validPersonalChannelNamespace bool
	if !usePersonalChannel || personalChannelNamespace == "" {
		validPersonalChannelNamespace = true
	}

	var nss = make([]string, 0, len(c.Namespaces))
	for _, n := range c.Namespaces {
		name := n.Name
		match := patternRegexp.MatchString(name)
		if !match {
			return errors.New(errPrefix + "wrong namespace name – " + name)
		}
		if stringInSlice(name, nss) {
			return errors.New(errPrefix + "namespace name must be unique")
		}
		if n.HistoryRecover && (n.HistorySize == 0 || n.HistoryLifetime == 0) {
			return fmt.Errorf("namespace %s: both history size and history lifetime required for history recovery", name)
		}
		if name == personalChannelNamespace {
			validPersonalChannelNamespace = true
		}
		nss = append(nss, name)
	}

	if !validPersonalChannelNamespace {
		return fmt.Errorf("namespace for user personal channel not found: %s", personalChannelNamespace)
	}

	return nil
}

type ChannelRuleContainer struct {
	mu     sync.RWMutex
	config ChannelRuleConfig
}

func NewNamespaceRuleContainer(config ChannelRuleConfig) *ChannelRuleContainer {
	return &ChannelRuleContainer{
		config: config,
	}
}

// Reload node config.
func (n *ChannelRuleContainer) Reload(c ChannelRuleConfig) error {
	if err := c.Validate(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config = c
	return nil
}

// ChannelOptions returns channel options for channel.
func (n *ChannelRuleContainer) ChannelOptions(ch string) (centrifuge.ChannelOptions, bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	opts, found, err := n.config.channelOpts(n.namespaceName(ch))
	if err != nil {
		return centrifuge.ChannelOptions{}, false, err
	}
	return opts.ChannelOptions, found, nil
}

// namespaceName returns namespace name from channel if exists.
func (n *ChannelRuleContainer) namespaceName(ch string) string {
	cTrim := strings.TrimPrefix(ch, n.config.TokenChannelPrefix)
	if n.config.ChannelNamespaceBoundary != "" && strings.Contains(cTrim, n.config.ChannelNamespaceBoundary) {
		parts := strings.SplitN(cTrim, n.config.ChannelNamespaceBoundary, 2)
		return parts[0]
	}
	return ""
}

// ChannelOpts returns channel options for channel using current channel config.
func (n *ChannelRuleContainer) NamespacedChannelOptions(ch string) (NamespaceChannelOptions, bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.channelOpts(n.namespaceName(ch))
}

// channelOpts searches for channel options for specified namespace key.
func (c *ChannelRuleConfig) channelOpts(namespaceName string) (NamespaceChannelOptions, bool, error) {
	if namespaceName == "" {
		return c.NamespaceChannelOptions, true, nil
	}
	for _, n := range c.Namespaces {
		if n.Name == namespaceName {
			return n.NamespaceChannelOptions, true, nil
		}
	}
	return NamespaceChannelOptions{}, false, nil
}

// PersonalChannel returns personal channel for user based on node configuration.
func (n *ChannelRuleContainer) PersonalChannel(user string) string {
	config := n.Config()
	if config.UserPersonalChannelNamespace == "" {
		return config.ChannelUserBoundary + user
	}
	return config.UserPersonalChannelNamespace + config.ChannelNamespaceBoundary + config.ChannelUserBoundary + user
}

// Config returns a copy of node Config.
func (n *ChannelRuleContainer) Config() ChannelRuleConfig {
	n.mu.RLock()
	c := n.config
	n.mu.RUnlock()
	return c
}

// IsTokenChannel checks if channel requires token to subscribe. In case of
// token-protected channel subscription request must contain a proper token.
func (n *ChannelRuleContainer) IsTokenChannel(ch string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.config.TokenChannelPrefix == "" {
		return false
	}
	return strings.HasPrefix(ch, n.config.TokenChannelPrefix)
}

// UserAllowed checks if user can subscribe on channel - as channel
// can contain special part in the end to indicate which users allowed
// to subscribe on it.
func (n *ChannelRuleContainer) UserAllowed(ch string, user string) bool {
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
