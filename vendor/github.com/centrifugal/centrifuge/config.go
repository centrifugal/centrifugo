package centrifuge

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Config contains Node configuration options.
type Config struct {
	// ChannelOptions embedded.
	ChannelOptions
	// Namespaces – list of namespaces for custom channel options.
	Namespaces []ChannelNamespace
	// Version of server – will be sent to client on connection establishment
	// phase in response to connect request.
	Version string
	// Name of this server node - must be unique, used as human readable
	// and meaningful node identifier.
	Name string
	// TokenHMACSecretKey is a secret key used to validate connection and subscription
	// tokens generated using HMAC. Zero value means that HMAC tokens won't be allowed.
	TokenHMACSecretKey string
	// UserPersonalChannelPrefix defines prefix to be added to user personal channel.
	UserPersonalChannelNamespace string
	// ChannelPrivatePrefix is a prefix in channel name which indicates that
	// channel is private.
	ChannelPrivatePrefix string
	// ChannelNamespaceBoundary is a string separator which must be put after
	// namespace part in channel name.
	ChannelNamespaceBoundary string
	// ChannelUserBoundary is a string separator which must be set before allowed
	// users part in channel name.
	ChannelUserBoundary string
	// ChannelUserSeparator separates allowed users in user part of channel name.
	ChannelUserSeparator string
	// TokenRSAPublicKey is a public key used to validate connection and subscription
	// tokens generated using RSA. Zero value means that RSA tokens won't be allowed.
	TokenRSAPublicKey *rsa.PublicKey
	// ClientPresencePingInterval is an interval how often connected clients
	// must update presence info.
	ClientPresencePingInterval time.Duration
	// ClientPresenceExpireInterval is an interval how long to consider
	// presence info valid after receiving presence ping.
	ClientPresenceExpireInterval time.Duration
	// ClientExpiredCloseDelay is an extra time given to client to
	// refresh its connection in the end of connection lifetime.
	ClientExpiredCloseDelay time.Duration
	// ClientExpiredSubCloseDelay is an extra time given to client to
	// refresh its expiring subscription in the end of subscription lifetime.
	ClientExpiredSubCloseDelay time.Duration
	// ClientStaleCloseDelay is a timeout after which connection will be
	// closed if still not authenticated (i.e. no valid connect command
	// received yet).
	ClientStaleCloseDelay time.Duration
	// ClientChannelPositionCheckDelay defines minimal time from previous
	// client position check in channel. If client does not pass check it will
	// be disconnected with DisconnectInsufficientState.
	ClientChannelPositionCheckDelay time.Duration
	// NodeInfoMetricsAggregateInterval sets interval for automatic metrics aggregation.
	// It's not very reasonable to have it less than one second.
	NodeInfoMetricsAggregateInterval time.Duration
	// LogLevel is a log level to use. By default nothing will be logged.
	LogLevel LogLevel
	// LogHandler is a handler func node will send logs to.
	LogHandler LogHandler
	// ClientQueueMaxSize is a maximum size of client's message queue in bytes.
	// After this queue size exceeded Centrifugo closes client's connection.
	ClientQueueMaxSize int
	// ClientChannelLimit sets upper limit of channels each client can subscribe to.
	ClientChannelLimit int
	// ClientUserConnectionLimit limits number of client connections from user with the
	// same ID. 0 - unlimited.
	ClientUserConnectionLimit int
	// ChannelMaxLength is a maximum length of channel name.
	ChannelMaxLength int
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
	// UserSubscribeToPersonal enables automatic subscribing to personal channel by user.
	// Only users with user ID defined will subscribe to personal channels, anonymous
	// users are ignored.
	UserSubscribeToPersonal bool
}

// Validate validates config and returns error if problems found
func (c *Config) Validate() error {
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

// channelOpts searches for channel options for specified namespace key.
func (c *Config) channelOpts(namespaceName string) (ChannelOptions, bool) {
	if namespaceName == "" {
		return c.ChannelOptions, true
	}
	for _, n := range c.Namespaces {
		if n.Name == namespaceName {
			return n.ChannelOptions, true
		}
	}
	return ChannelOptions{}, false
}

const (
	// nodeInfoPublishInterval is an interval how often node must publish
	// node control message.
	nodeInfoPublishInterval = 3 * time.Second
	// nodeInfoCleanInterval is an interval in seconds, how often node must
	// clean information about other running nodes.
	nodeInfoCleanInterval = nodeInfoPublishInterval * 3
	// nodeInfoMaxDelay is an interval in seconds – how many seconds node
	// info considered actual.
	nodeInfoMaxDelay = nodeInfoPublishInterval*2 + time.Second
)

// DefaultConfig is Config initialized with default values for all fields.
var DefaultConfig = Config{
	Name: "centrifuge",

	NodeInfoMetricsAggregateInterval: 60 * time.Second,

	ChannelMaxLength:         255,
	ChannelPrivatePrefix:     "$", // so private channel will look like "$gossips"
	ChannelNamespaceBoundary: ":", // so namespace "public" can be used as "public:news"
	ChannelUserBoundary:      "#", // so user limited channel is "user#2694" where "2696" is user ID
	ChannelUserSeparator:     ",", // so several users limited channel is "dialog#2694,3019"

	ClientPresencePingInterval:      25 * time.Second,
	ClientPresenceExpireInterval:    60 * time.Second,
	ClientExpiredCloseDelay:         25 * time.Second,
	ClientExpiredSubCloseDelay:      25 * time.Second,
	ClientStaleCloseDelay:           25 * time.Second,
	ClientChannelPositionCheckDelay: 40 * time.Second,
	ClientQueueMaxSize:              10485760, // 10MB by default
	ClientChannelLimit:              128,
}
