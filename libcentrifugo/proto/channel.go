package proto

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

	// Presence turns on(off) presence information for channels. Presence is a structure with
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

	// HistoryDropInactive enables an optimization where history is only saved for channels that have at
	// least one active subscriber. This can give a huge memory saving, with only minor edgecases that are
	// different from without it as noted on https://github.com/centrifugal/centrifugo/issues/50.
	HistoryDropInactive bool `mapstructure:"history_drop_inactive" json:"history_drop_inactive"`
}
