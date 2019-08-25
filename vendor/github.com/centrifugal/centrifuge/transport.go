package centrifuge

import (
	"net/http"
)

// TransportMeta contains extended transport description.
// Depending on transport implementation some fields here can be missing.
type TransportMeta struct {
	// Request contains initial HTTP request sent by client. Can be nil in case of
	// non-HTTP based transports. Though both Websocket and SockJS we currently
	// support use HTTP on start so this field will present.
	Request *http.Request
}

// TransportInfo has read-only transport description methods.
type TransportInfo interface {
	// Name returns a name of transport used for client connection.
	Name() string
	// Protocol returns underlying transport protocol type used.
	// At moment this can be for example a JSON streaming based protocol
	// or Protobuf length-delimited protocol.
	Protocol() ProtocolType
	// Encoding returns payload encoding type used by client. By default
	// server assumes that payload passed as JSON.
	Encoding() EncodingType
	// Meta returns transport meta information.
	Meta() TransportMeta
}

// Transport abstracts a connection transport between server and client.
// It does not contain Read method as reading can be handled by connection
// handler code.
type Transport interface {
	TransportInfo
	// Send sends data encoded using Centrifuge protocol to connection.
	Write([]byte) error
	// Close closes transport.
	Close(*Disconnect) error
}
