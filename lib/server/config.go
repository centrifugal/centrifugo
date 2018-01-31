package server

// Config contains HTTPServer configuration options.
type Config struct {
	// APIKey allows to protect API handler with API key authorization.
	// This auth method makes sense when you deploy Centrifugo with TLS enabled.
	// Otherwise we must strongly advice users protect API endpoint with firewall.
	APIKey string `json:"api_key"`

	// APIInsecure turns off API key check.
	// This can be useful if API endpoint protected with firewall or someone wants
	// to play with API (for example from command line using CURL).
	APIInsecure bool `json:"api_insecure"`

	// AdminPassword is an admin password.
	AdminPassword string `json:"admin_password"`

	// AdminSecret is a secret to generate auth token for admin socket connection.
	AdminSecret string `json:"admin_secret"`

	// AdminInsecure turns on insecure mode for admin endpoints - no auth required to
	// connect to admin socket and web interface. Protect admin resources with firewall
	// rules in production when enabling this option.
	AdminInsecure bool `json:"admin_insecure"`

	// WebsocketCompression allows to enable websocket permessage-deflate
	// compression support for raw websocket connections. It does not guarantee
	// that compression will be used - i.e. it only says that Centrifugo will
	// try to negotiate it with client.
	WebsocketCompression bool `json:"websocket_compression"`

	// WebsocketCompressionLevel sets a level for websocket compression.
	// See posiible value description at https://golang.org/pkg/compress/flate/#NewWriter
	WebsocketCompressionLevel int `json:"websocket_compression_level"`

	// WebsocketCompressionMinSize allows to set minimal limit in bytes for message to use
	// compression when writing it into client connection. By default it's 0 - i.e. all messages
	// will be compressed when WebsocketCompression enabled and compression negotiated with client.
	WebsocketCompressionMinSize int `json:"websocket_compression_min_size"`

	// WebsocketReadBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketReadBufferSize int `json:"websocket_read_buffer_size"`

	// WebsocketWriteBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketWriteBufferSize int `json:"websocket_write_buffer_size"`
}
