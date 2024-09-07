package uniws

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults.
const (
	DefaultWebsocketPingInterval     = 25 * time.Second
	DefaultWebsocketWriteTimeout     = 1 * time.Second
	DefaultWebsocketMessageSizeLimit = 65536 // 64KB
)

// Config represents config for Handler.
type Config struct {
	Enabled            bool          `mapstructure:"enabled" json:"enabled" envconfig:"enabled"`
	HandlerPrefix      string        `mapstructure:"handler_prefix" json:"handler_prefix" envconfig:"handler_prefix" default:"/connection/uni_websocket"`
	Compression        bool          `mapstructure:"compression" json:"compression" envconfig:"compression"`
	CompressionMinSize int           `mapstructure:"compression_min_size" json:"compression_min_size" envconfig:"compression_min_size"`
	CompressionLevel   int           `mapstructure:"compression_level" json:"compression_level" envconfig:"compression_level" default:"1"`
	ReadBufferSize     int           `mapstructure:"read_buffer_size" json:"read_buffer_size" envconfig:"read_buffer_size"`
	UseWriteBufferPool bool          `mapstructure:"use_write_buffer_pool" json:"use_write_buffer_pool" envconfig:"use_write_buffer_pool"`
	WriteBufferSize    int           `mapstructure:"write_buffer_size" json:"write_buffer_size" envconfig:"write_buffer_size"`
	WriteTimeout       time.Duration `mapstructure:"write_timeout" json:"write_timeout" envconfig:"write_timeout" default:"1000ms"`
	MessageSizeLimit   int           `mapstructure:"message_size_limit" json:"message_size_limit" envconfig:"message_size_limit" default:"65536"`
}

func sameHostOriginCheck() func(r *http.Request) bool {
	return func(r *http.Request) bool {
		err := checkSameHost(r)
		return err == nil
	}
}

func checkSameHost(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("failed to parse Origin header %q: %w", origin, err)
	}
	if strings.EqualFold(r.Host, u.Host) {
		return nil
	}
	return fmt.Errorf("request Origin %q is not authorized for Host %q", origin, r.Host)
}
