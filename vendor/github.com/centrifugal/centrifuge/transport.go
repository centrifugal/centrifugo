package centrifuge

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
