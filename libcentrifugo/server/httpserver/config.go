package httpserver

import (
	"errors"

	"github.com/centrifugal/centrifugo/libcentrifugo/config"
)

// Config contains Application configuration options.
type Config struct {

	// Web enables admin web interface.
	Web bool `json:"web"`
	// WebPath
	WebPath string `json:"web_path"`

	// HTTPAddress
	HTTPAddress string `json:"http_address"`
	// HTTPPrefix
	HTTPPrefix string `json:"http_prefix"`
	// HTTPPort
	HTTPPort string `json:"http_port"`
	// HTTPAdminPort
	HTTPAdminPort string `json:"http_admin_port"`
	// HTTPAPIPort
	HTTPAPIPort string `json:"http_api_port"`

	// SSL enables builtin https server.
	SSL bool `json:"ssl"`
	// SSLCert is path to SSL certificate file.
	SSLCert string `json:"ssl_cert"`
	// SSLKey is path to SSL key file.
	SSLKey string `json:"ssl_key"`

	// SockjsURL is a custom SockJS library url to use in iframe transports.
	SockjsURL string `json:"sockjs_url"`

	// WebsocketCompression allows to enable websocket permessage-deflate
	// compression support for raw websocket connections. It does not guarantee
	// that compression will be used - i.e. it only says that Centrifugo will
	// try to negotiate it with client.
	WebsocketCompression bool `json:"websocket_compression"`

	// WebsocketReadBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketReadBufferSize int `json:"websocket_read_buffer_size"`

	// WebsocketWriteBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WebsocketWriteBufferSize int `json:"websocket_write_buffer_size"`
}

// newConfig creates new libcentrifugo.Config using viper.
func newConfig(c config.Getter) *Config {
	cfg := &Config{}
	cfg.Web = c.GetBool("web")
	cfg.WebPath = c.GetString("web_path")
	cfg.HTTPAddress = c.GetString("address")
	cfg.HTTPPort = c.GetString("port")
	cfg.HTTPAdminPort = c.GetString("admin_port")
	cfg.HTTPAPIPort = c.GetString("api_port")
	cfg.HTTPPrefix = c.GetString("http_prefix")
	cfg.SockjsURL = c.GetString("sockjs_url")
	cfg.SSL = c.GetBool("ssl")
	cfg.SSLCert = c.GetString("ssl_cert")
	cfg.SSLKey = c.GetString("ssl_cert")
	cfg.WebsocketCompression = c.GetBool("websocket_compression")
	cfg.WebsocketReadBufferSize = c.GetInt("websocket_read_buffer_size")
	cfg.WebsocketWriteBufferSize = c.GetInt("websocket_write_buffer_size")
	return cfg
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

	if c.SSL {
		if c.SSLCert == "" {
			return errors.New(errPrefix + "no SSL certificate provided")
		}
		if c.SSLKey == "" {
			return errors.New(errPrefix + "no SSL certificate key provided")
		}
	}
	return nil
}

// DefaultConfig is Config initialized with default values for all fields.
var DefaultConfig = &Config{}
