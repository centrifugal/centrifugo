package node

import (
	"errors"
	"regexp"
	"time"

	"github.com/centrifugal/centrifugo/lib/channel"
)

// Config contains Application configuration options.
type Config struct {
	// Name of this server node - must be unique, used as human readable
	// and meaningful node identificator.
	Name string `json:"name"`

	// Admin enables admin socket.
	Admin bool `json:"admin"`
	// AdminPassword is an admin password.
	AdminPassword string `json:"-"`
	// AdminSecret is a secret to generate auth token for admin socket connection.
	AdminSecret string `json:"-"`

	// Insecure turns on insecure mode - when it's turned on then no authentication
	// required at all when connecting to Centrifugo, anonymous access and publish
	// allowed for all channels, no connection check performed. This can be suitable
	// for demonstration or personal usage.
	Insecure bool `json:"insecure"`
	// InsecureAPI turns on insecure mode for HTTP API calls. This means that no
	// API sign required when sending commands. This can be useful if you don't want
	// to sign every request - for example if you closed API endpoint with firewall
	// or you want to play with API commands from command line using CURL.
	InsecureAPI bool `json:"insecure_api"`
	// InsecureAdmin turns on insecure mode for admin endpoints - no auth required to
	// connect to admin socket and web interface. Protect admin resources with firewall
	// rules in production when enabling this option.
	InsecureAdmin bool `json:"insecure_admin"`

	// MaxChannelLength is a maximum length of channel name.
	MaxChannelLength int `json:"max_channel_length"`

	// PingInterval sets interval server will send ping messages to clients.
	PingInterval time.Duration `json:"ping_interval"`

	// NodePingInterval is an interval how often node must send ping
	// control message.
	NodePingInterval time.Duration `json:"node_ping_interval"`
	// NodeInfoCleanInterval is an interval in seconds, how often node must clean
	// information about other running nodes.
	NodeInfoCleanInterval time.Duration `json:"node_info_clean_interval"`
	// NodeInfoMaxDelay is an interval in seconds – how many seconds node info
	// considered actual.
	NodeInfoMaxDelay time.Duration `json:"node_info_max_delay"`
	// NodeMetricsInterval detects interval node will use to aggregate metrics.
	NodeMetricsInterval time.Duration `json:"node_metrics_interval"`

	// PresencePingInterval is an interval how often connected clients
	// must update presence info.
	PresencePingInterval time.Duration `json:"presence_ping_interval"`
	// PresenceExpireInterval is an interval how long to consider
	// presence info valid after receiving presence ping.
	PresenceExpireInterval time.Duration `json:"presence_expire_interval"`

	// ExpiredConnectionCloseDelay is an interval given to client to
	// refresh its connection in the end of connection lifetime.
	ExpiredConnectionCloseDelay time.Duration `json:"expired_connection_close_delay"`

	// StaleConnectionCloseDelay is an interval in seconds after which
	// connection will be closed if still not authenticated.
	StaleConnectionCloseDelay time.Duration `json:"stale_connection_close_delay"`

	// MessageWriteTimeout is maximum time of write message operation.
	// Slow client will be disconnected. By default we don't use this option (i.e. it's 0)
	// and slow client connections will be closed when there queue size exceeds
	// ClientQueueMaxSize. In case of SockJS transport we don't have control over it so
	// it only affects raw websocket.
	ClientMessageWriteTimeout time.Duration `json:"client_message_write_timeout"`

	// ClientRequestMaxSize sets maximum size in bytes of allowed client request.
	ClientRequestMaxSize int `json:"client_request_max_size"`
	// ClientQueueMaxSize is a maximum size of client's message queue in bytes.
	// After this queue size exceeded Centrifugo closes client's connection.
	ClientQueueMaxSize int `json:"client_queue_max_size"`
	// ClientQueueInitialCapacity sets initial amount of slots in client message
	// queue. When these slots are full client queue is automatically resized to
	// a bigger size. This option can reduce amount of allocations when message
	// rate is very high and client queue resizes frequently. Note that memory
	// consumption per client connection grows with this option.
	ClientQueueInitialCapacity int `json:"client_queue_initial_capacity"`

	// ClientChannelLimit sets upper limit of channels each client can subscribe to.
	ClientChannelLimit int `json:"client_channel_limit"`

	// UserConnectionLimit limits number of connections from user with the same ID.
	UserConnectionLimit int `json:"user_connection_limit"`

	// PrivateChannelPrefix is a prefix in channel name which indicates that
	// channel is private.
	PrivateChannelPrefix string `json:"private_channel_prefix"`
	// NamespaceChannelBoundary is a string separator which must be put after
	// namespace part in channel name.
	NamespaceChannelBoundary string `json:"namespace_channel_boundary"`
	// UserChannelBoundary is a string separator which must be set before allowed
	// users part in channel name.
	UserChannelBoundary string `json:"user_channel_boundary"`
	// UserChannelSeparator separates allowed users in user part of channel name.
	UserChannelSeparator string `json:"user_channel_separator"`
	// ClientChannelBoundary is a string separator which must be set before client
	// connection ID in channel name so only client with this ID can subscribe on
	// that channel.
	ClientChannelBoundary string `json:"client_channel_separator"`

	// Secret is a secret key, used to sign API requests and client connection tokens.
	Secret string `json:"secret"`

	// ConnLifetime determines time until connection expire, 0 means no connection expire at all.
	ConnLifetime int64 `json:"connection_lifetime"`

	// channel.Options embedded to config.
	channel.Options `json:"channel_options"`

	// Namespaces - list of namespaces for custom channel options.
	Namespaces []channel.Namespace `json:"namespaces"`
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
func (c *Config) channelOpts(nk channel.NamespaceKey) (channel.Options, bool) {
	if nk == channel.NamespaceKey("") {
		return c.Options, false
	}
	for _, n := range c.Namespaces {
		if n.Name == nk {
			return n.Options, true
		}
	}
	return channel.Options{}, false
}

const (
	// DefaultName of node.
	DefaultName = "centrifugo"
	// DefaultNodePingInterval used in default config.
	DefaultNodePingInterval = 3
)

// DefaultConfig is Config initialized with default values for all fields.
var DefaultConfig = &Config{
	Name:                        DefaultName,
	Admin:                       false,
	AdminPassword:               "",
	AdminSecret:                 "",
	Insecure:                    false,
	InsecureAPI:                 false,
	InsecureAdmin:               false,
	MaxChannelLength:            255,
	PingInterval:                25 * time.Second,
	NodePingInterval:            DefaultNodePingInterval * time.Second,
	NodeInfoCleanInterval:       DefaultNodePingInterval * 3 * time.Second,
	NodeInfoMaxDelay:            DefaultNodePingInterval*2*time.Second + 1*time.Second,
	NodeMetricsInterval:         60 * time.Second,
	PresencePingInterval:        25 * time.Second,
	PresenceExpireInterval:      60 * time.Second,
	ClientMessageWriteTimeout:   0,
	PrivateChannelPrefix:        "$", // so private channel will look like "$gossips"
	NamespaceChannelBoundary:    ":", // so namespace "public" can be used "public:news"
	ClientChannelBoundary:       "&", // so client channel is sth like "client&7a37e561-c720-4608-52a8-a964a9db7a8a"
	UserChannelBoundary:         "#", // so user limited channel is "user#2694" where "2696" is user ID
	UserChannelSeparator:        ",", // so several users limited channel is "dialog#2694,3019"
	ExpiredConnectionCloseDelay: 25 * time.Second,
	StaleConnectionCloseDelay:   25 * time.Second,
	ClientRequestMaxSize:        65536,    // 64KB by default
	ClientQueueMaxSize:          10485760, // 10MB by default
	ClientQueueInitialCapacity:  2,
	ClientChannelLimit:          128,
}
