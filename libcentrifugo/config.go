package libcentrifugo

import (
	"errors"
	"regexp"
	"time"
)

// ChannelOptions represent channel specific configuration for namespace or project in a whole
type ChannelOptions struct {
	// Watch determines if message published into channel will be also sent into admin channel.
	// Note that this option must be used carefully in channels with high rate of new messages
	// as admin client can not process all of those messages. Use this option for testing or for
	// channels with reasonable message rate.
	Watch bool `json:"watch"`

	// Publish determines if client can publish messages into channel directly. This allows to use
	// Centrifugo without backend. All messages go through Centrifugo and delivered to clients. But
	// in this case you lose everything your backend code could give - validation, persistence etc.
	// This option most useful for demos, testing real-time ideas.
	Publish bool `json:"publish"`

	// Anonymous determines is anonymous access (with empty user ID) allowed or not. In most
	// situations your application works with authorized users so every user has its own unique
	// id. But if you provide real-time features for public access you may need anauthorized
	// access to channels. Turn on this option and use empty string as user ID.
	Anonymous bool `json:"anonymous"`

	// Presence turns on(off) presence information for channels. Presense is a structure with
	// clients currently subscribed on channel.
	Presence bool `json:"presence"`

	// JoinLeave turns on(off) join/leave messages for channels. When client subscribes on channel
	// join message sent to all clients in this channel. When client leaves channel (unsubscribes)
	// leave message sent.
	JoinLeave bool `mapstructure:"join_leave" json:"join_leave"`

	// HistorySize determines max amount of history messages for channel, 0 means no history for channel.
	// Centrifugo history has auxiliary role – it can not replace your backend persistent storage.
	HistorySize int `mapstructure:"history_size" json:"history_size"`

	// HistoryLifetime determines time in seconds until expiration for history messages. As Centrifugo
	// keeps history in memory (in process memory or in Redis process memory) it's important to remove
	// old messages to prevent infinite memory grows.
	HistoryLifetime int `mapstructure:"history_lifetime" json:"history_lifetime"`

	// Recover enables recover mechanism for channels. This means that Centrifugo will
	// try to recover missed messages for resubscribing client. This option uses messages
	// from history and must be used with reasonable HistorySize and HistoryLifetime
	// configuration.
	Recover bool `json:"recover"`
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

	// Web shows if admin web interface enabled or not
	Web bool
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

	// StaleConnectionCloseDelay is an interval in seconds after which
	// connection will be closed if still not authenticated.
	StaleConnectionCloseDelay time.Duration

	// MessageSendTimeout is an interval how long time the node
	// may take to send a message to a client before disconnecting the client.
	MessageSendTimeout time.Duration

	// MaxClientQueueSize is a maximum size of client's message queue in bytes.
	// After this queue size exceeded Centrifugo closes client's connection.
	MaxClientQueueSize int

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
	// for demonstration or personal usage.
	Insecure bool
	// InsecureAPI turns on insecure mode for HTTP API calls. This means that no
	// API sign required when sending commands. This can be useful if you don't want
	// to sign every request - for example if you closed API endpoint with firewall
	// or you want to play with API commands from command line using CURL.
	InsecureAPI bool
	// InsecureWeb turns on insecure mode for admin web interface handler endpoints. This
	// means that no web password and web secret will be used to protect access to web admin
	// resources. Use this in development or protect admin resources with firewall rules
	// in production.
	InsecureWeb bool

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
	}
	for _, n := range c.Namespaces {
		if n.Name == nk {
			return n.ChannelOptions, nil
		}
	}
	return ChannelOptions{}, ErrNamespaceNotFound
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
	ExpiredConnectionCloseDelay: 25 * time.Second,
	StaleConnectionCloseDelay:   25 * time.Second,
	MaxClientQueueSize:          10485760, // 10MB by default
	Insecure:                    false,
}
