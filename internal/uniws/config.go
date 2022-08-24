package uniws

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/centrifugal/centrifuge"
)

// Defaults.
const (
	DefaultWebsocketPingInterval     = 25 * time.Second
	DefaultWebsocketWriteTimeout     = 1 * time.Second
	DefaultWebsocketMessageSizeLimit = 65536 // 64KB
)

// Config represents config for Handler.
type Config struct {
	// ProtocolVersion used by default. If not set then we use centrifuge.ProtocolVersion1.
	ProtocolVersion centrifuge.ProtocolVersion

	// CompressionLevel sets a level for websocket compression.
	// See possible value description at https://golang.org/pkg/compress/flate/#NewWriter
	CompressionLevel int

	// CompressionMinSize allows to set minimal limit in bytes for
	// message to use compression when writing it into client connection.
	// By default, it's 0 - i.e. all messages will be compressed when
	// WebsocketCompression enabled and compression negotiated with client.
	CompressionMinSize int

	// ReadBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	ReadBufferSize int

	// WriteBufferSize is a parameter that is used for raw websocket Upgrader.
	// If set to zero reasonable default value will be used.
	WriteBufferSize int

	// MessageSizeLimit sets the maximum size in bytes of allowed message from client.
	// By default, DefaultWebsocketMaxMessageSize will be used.
	MessageSizeLimit int

	// CheckOrigin func to provide custom origin check logic.
	// nil means allow all origins.
	CheckOrigin func(r *http.Request) bool

	// PingInterval sets interval server will send ping messages to clients.
	// By default DefaultPingInterval will be used.
	PingInterval time.Duration

	// WriteTimeout is maximum time of write message operation.
	// Slow client will be disconnected.
	// By default, DefaultWebsocketWriteTimeout will be used.
	WriteTimeout time.Duration

	// Compression allows to enable websocket permessage-deflate
	// compression support for raw websocket connections. It does
	// not guarantee that compression will be used - i.e. it only
	// says that server will try to negotiate it with client.
	Compression bool

	// UseWriteBufferPool enables using buffer pool for writes.
	UseWriteBufferPool bool

	centrifuge.PingPongConfig
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
