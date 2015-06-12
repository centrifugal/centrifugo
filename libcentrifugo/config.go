package libcentrifugo

// Config contains Application configuration options.
type Config struct {
	// Version is a version of node as string, in most cases this will
	// be Centrifugo server version.
	Version string

	// Name of this node - must be unique, used as human readable and
	// meaningful node identificator.
	Name string

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

	// NodePingInterval is an interval in seconds, how often node must send ping
	// control message.
	NodePingInterval int64
	// NodeInfoCleanInterval is an interval in seconds, how often node must clean
	// information about other running nodes.
	NodeInfoCleanInterval int64
	// NodeInfoMaxDelay is an interval in seconds – how many seconds node info
	// considered actual.
	NodeInfoMaxDelay int64

	// PresencePingInterval is an interval in seconds – how often connected clients
	// must update presence info.
	PresencePingInterval int64
	// PresenceExpireInterval is an interval in seconds – how long to consider
	// presence info valid after receiving presence ping.
	PresenceExpireInterval int64

	// ExpiredConnectionCloseDelay is an interval in seconds given to client to
	// refresh its connection in the end of connection lifetime.
	ExpiredConnectionCloseDelay int64

	// MessageSendTimeout is an interval in seconds setting how long time the node
	// may take to send a message to a client before disconnecting the client.
	MessageSendTimeout int64

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

	// Insecure turns on insecure mode - when it's turned on then no authentication
	// required at all when connecting to Centrifugo, anonymous access and publish
	// allowed for all channels, no connection check performed. This can be suitable
	// for demonstration or personal usage
	Insecure bool
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
	WebPassword:                 "",
	WebSecret:                   "",
	ChannelPrefix:               defaultChannelPrefix,
	AdminChannel:                ChannelID(defaultChannelPrefix + ".admin"),
	ControlChannel:              ChannelID(defaultChannelPrefix + ".control"),
	MaxChannelLength:            255,
	NodePingInterval:            int64(defaultNodePingInterval),
	NodeInfoCleanInterval:       int64(defaultNodePingInterval) * 3,
	NodeInfoMaxDelay:            int64(defaultNodePingInterval)*2 + 1,
	PresencePingInterval:        int64(25),
	PresenceExpireInterval:      int64(60),
	MessageSendTimeout:          int64(60),
	PrivateChannelPrefix:        "$",
	NamespaceChannelBoundary:    ":",
	UserChannelBoundary:         "#",
	UserChannelSeparator:        ",",
	ExpiredConnectionCloseDelay: int64(10),
	Insecure:                    false,
}
