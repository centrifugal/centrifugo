package libcentrifugo

import (
	"errors"
	"regexp"
	"time"
)

// ChannelOptions represent channel specific configuration for namespace or project in a whole
type ChannelOptions struct {
	// Watch determines if message published into channel will be sent into admin channel
	Watch bool `json:"watch"`

	// Publish determines if client can publish messages into channel directly
	Publish bool `json:"publish"`

	// Anonymous determines is anonymous access (with empty user ID) allowed or not
	Anonymous bool `json:"anonymous"`

	// Presence turns on(off) presence information for channels
	Presence bool `json:"presence"`

	// HistorySize determines max amount of history messages for channel, 0 means no history for channel
	HistorySize int `mapstructure:"history_size" json:"history_size"`

	// HistoryLifetime determines time in seconds until expiration for history messages
	HistoryLifetime int `mapstructure:"history_lifetime" json:"history_lifetime"`

	// JoinLeave turns on(off) join/leave messages for channels
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave"`
}

// NamespaceKey is a name of namespace unique for project.
type NamespaceKey string

// Namespace allows to create channels with different channel options within the Project
type Namespace struct {
	// Name is a unique namespace name.
	Name NamespaceKey `json:"name"`

	// ChannelOptions for namespace determine channel options for channels belonging to this namespace.
	ChannelOptions `mapstructure:",squash"`
}

// Config contains Application configuration options.
type Config struct {
	// Version is a version of node as string, in most cases this will
	// be Centrifugo server version.
	Version string

	// Name of this node - must be unique, used as human readable and
	// meaningful node identificator.
	Name string

	// Debug turns on application debug mode.
	Debug bool

	// WebPassword is an admin web interface password.
	WebPassword string
	// WebSecret is a secret to generate auth token for admin web interface.
	WebSecret string

	// ChannelPrefix is a string prefix before each channel.
	ChannelPrefix string
	// AdminChannel is channel name for admin messages.
	AdminChannel ChannelID
	// ControlChannel is a channel name for internal control messages between nodes.
	ControlChannel ChannelID
	// MaxChannelLength is a maximum length of channel name.
	MaxChannelLength int

	// PingInterval sets interval server will send ping messages to clients.
	PingInterval time.Duration

	// NodePingInterval is an interval how often node must send ping
	// control message.
	NodePingInterval time.Duration
	// NodeInfoCleanInterval is an interval in seconds, how often node must clean
	// information about other running nodes.
	NodeInfoCleanInterval time.Duration
	// NodeInfoMaxDelay is an interval in seconds – how many seconds node info
	// considered actual.
	NodeInfoMaxDelay time.Duration
	// NodeMetricsInterval detects interval node will use to aggregate metrics.
	NodeMetricsInterval time.Duration

	// PresencePingInterval is an interval how often connected clients
	// must update presence info.
	PresencePingInterval time.Duration
	// PresenceExpireInterval is an interval how long to consider
	// presence info valid after receiving presence ping.
	PresenceExpireInterval time.Duration

	// ExpiredConnectionCloseDelay is an interval given to client to
	// refresh its connection in the end of connection lifetime.
	ExpiredConnectionCloseDelay time.Duration

	// MessageSendTimeout is an interval how long time the node
	// may take to send a message to a client before disconnecting the client.
	MessageSendTimeout time.Duration

	// PrivateChannelPrefix is a prefix in channel name which indicates that
	// channel is private.
	PrivateChannelPrefix string
	// NamespaceChannelBoundary is a string separator which must be put after
	// namespace part in channel name.
	NamespaceChannelBoundary string
	// UserChannelBoundary is a string separator which must be set before allowed
	// users part in channel name.
	UserChannelBoundary string
	// UserChannelSeparator separates allowed users in user part of channel name.
	UserChannelSeparator string
	// ClientChannelBoundary is a string separator which must be set before client
	// connection ID in channel name so only client with this ID can subscribe on
	// that channel.
	ClientChannelBoundary string

	// Insecure turns on insecure mode - when it's turned on then no authentication
	// required at all when connecting to Centrifugo, anonymous access and publish
	// allowed for all channels, no connection check performed. This can be suitable
	// for demonstration or personal usage
	Insecure bool

	// Secret is a secret key, used to sign API requests and client connection tokens.
	Secret string

	// ConnLifetime determines time until connection expire, 0 means no connection expire at all.
	ConnLifetime int64

	// ChannelOptions embedded to config.
	ChannelOptions

	// Namespaces - list of namespaces for custom channel options.
	Namespaces []Namespace
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
	errPrefix := "config error: "
	pattern := "^[-a-zA-Z0-9_]{2,}$"

	if c.Secret == "" {
		return errors.New(errPrefix + "secret required")
	}

	var nss []string
	for _, n := range c.Namespaces {
		name := string(n.Name)
		match, _ := regexp.MatchString(pattern, name)
		if !match {
			return errors.New(errPrefix + "wrong namespace name – " + name)
		}
		if stringInSlice(name, nss) {
			return errors.New(errPrefix + "namespace name must be unique")
		}
		nss = append(nss, name)
	}

	return nil
}

// channelOpts searches for channel options for specified namespace key.
func (c *Config) channelOpts(nk NamespaceKey) (ChannelOptions, error) {
	if nk == NamespaceKey("") {
		return c.ChannelOptions, nil
	} else {
		for _, n := range c.Namespaces {
			if n.Name == nk {
				return n.ChannelOptions, nil
			}
		}
		return ChannelOptions{}, ErrNamespaceNotFound
	}
}

const (
	defaultName             = "libcentrifugo"
	defaultChannelPrefix    = "libcentrifugo"
	defaultNodePingInterval = 5
)

// DefaultConfig is Config initialized with default values for all fields.
var DefaultConfig = &Config{
	Version:                     "-",
	Name:                        defaultName,
	Debug:                       false,
	WebPassword:                 "",
	WebSecret:                   "",
	ChannelPrefix:               defaultChannelPrefix,
	AdminChannel:                ChannelID(defaultChannelPrefix + ".admin"),
	ControlChannel:              ChannelID(defaultChannelPrefix + ".control"),
	MaxChannelLength:            255,
	PingInterval:                25 * time.Second,
	NodePingInterval:            defaultNodePingInterval * time.Second,
	NodeInfoCleanInterval:       defaultNodePingInterval * 3 * time.Second,
	NodeInfoMaxDelay:            defaultNodePingInterval*2*time.Second + 1*time.Second,
	NodeMetricsInterval:         60 * time.Second,
	PresencePingInterval:        25 * time.Second,
	PresenceExpireInterval:      60 * time.Second,
	MessageSendTimeout:          0,
	PrivateChannelPrefix:        "$", // so private channel will look like "$gossips"
	NamespaceChannelBoundary:    ":", // so namespace "public" can be used "public:news"
	ClientChannelBoundary:       "&", // so client channel is sth like "client&7a37e561-c720-4608-52a8-a964a9db7a8a"
	UserChannelBoundary:         "#", // so user limited channel is "user#2694" where "2696" is user ID
	UserChannelSeparator:        ",", // so several users limited channel is "dialog#2694,3019"
	ExpiredConnectionCloseDelay: 10 * time.Second,
	Insecure:                    false,
}
