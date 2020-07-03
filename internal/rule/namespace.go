package rule

import "github.com/centrifugal/centrifuge"

var ErrorNamespaceNotFound = &centrifuge.Error{
	Code:    4000,
	Message: "namespace not found",
}

// ChannelNamespace allows to create channels with different channel options.
type ChannelNamespace struct {
	// Name is a unique namespace name.
	Name string `json:"name"`

	// Options for namespace determine channel options for channels
	// belonging to this namespace.
	NamespaceChannelOptions `mapstructure:",squash"`
}

// ChannelOptions represent channel specific configuration for namespace
// or global channel options if set on top level of configuration.
type NamespaceChannelOptions struct {
	centrifuge.ChannelOptions `mapstructure:",squash"`

	// ServerSide marks all channels in namespace as server side, when on then client
	// subscribe requests to these channels will be rejected with PermissionDenied error.
	ServerSide bool `mapstructure:"server_side" json:"server_side"`

	// Publish enables possibility for clients to publish messages into channels.
	// Once enabled client can publish into channel and that publication will be
	// broadcasted to all current channel subscribers. You can control publishing
	// on server-side setting On().Publish callback to client connection.
	Publish bool `json:"publish"`

	// SubscribeToPublish turns on an automatic check that client subscribed
	// on channel before allow it to publish into that channel.
	SubscribeToPublish bool `mapstructure:"subscribe_to_publish" json:"subscribe_to_publish"`

	// Anonymous enables anonymous access (with empty user ID) to channel.
	// In most situations your application works with authenticated users so
	// every user has its own unique user ID. But if you provide real-time
	// features for public access you may need unauthenticated access to channels.
	// Turn on this option and use empty string as user ID.
	Anonymous bool `json:"anonymous"`

	// PresenceDisableForClient prevents presence to be asked by clients.
	// In this case it's available only over server-side presence call.
	PresenceDisableForClient bool `mapstructure:"presence_disable_for_client" json:"presence_disable_for_client"`

	// HistoryDisableForClient prevents history to be asked by clients.
	// In this case it's available only over server-side history call.
	// History recover mechanism if enabled will continue to work for
	// clients anyway.
	HistoryDisableForClient bool `mapstructure:"history_disable_for_client" json:"history_disable_for_client"`
}
